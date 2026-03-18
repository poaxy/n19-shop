package payment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"remnawave-tg-shop-bot/internal/cache"
	"remnawave-tg-shop-bot/internal/ctxkeys"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/cryptopay"
	"remnawave-tg-shop-bot/internal/database"
	"remnawave-tg-shop-bot/internal/domain"
	"remnawave-tg-shop-bot/internal/moynalog"
	"remnawave-tg-shop-bot/internal/remnawave"
	"remnawave-tg-shop-bot/internal/stripe"
	"remnawave-tg-shop-bot/internal/translation"
	"remnawave-tg-shop-bot/internal/yookasa"
	"remnawave-tg-shop-bot/utils"
	"time"

	remapi "github.com/Jolymmiles/remnawave-api-go/v2/api"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type PaymentService struct {
	purchaseRepository *database.PurchaseRepository
	remnawaveClient    *remnawave.Client
	customerRepository *database.CustomerRepository
	telegramBot        *bot.Bot
	translation        *translation.Manager
	cryptoPayClient    *cryptopay.Client
	yookasaClient      *yookasa.Client
	referralRepository *database.ReferralRepository
	cache              *cache.Cache
	moynalogClient     *moynalog.Client
	stripeClient       *stripe.Client
}

func NewPaymentService(
	translation *translation.Manager,
	purchaseRepository *database.PurchaseRepository,
	remnawaveClient *remnawave.Client,
	customerRepository *database.CustomerRepository,
	telegramBot *bot.Bot,
	cryptoPayClient *cryptopay.Client,
	yookasaClient *yookasa.Client,
	referralRepository *database.ReferralRepository,
	cache *cache.Cache,
	moynalogClient *moynalog.Client,
	stripeClient *stripe.Client,
) *PaymentService {
	return &PaymentService{
		purchaseRepository: purchaseRepository,
		remnawaveClient:    remnawaveClient,
		customerRepository: customerRepository,
		telegramBot:        telegramBot,
		translation:        translation,
		cryptoPayClient:    cryptoPayClient,
		yookasaClient:      yookasaClient,
		referralRepository: referralRepository,
		cache:              cache,
		moynalogClient:     moynalogClient,
		stripeClient:       stripeClient,
	}
}

func (s PaymentService) ProcessPurchaseById(ctx context.Context, purchaseId int64) error {
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseId)
	if err != nil {
		return err
	}
	if purchase == nil {
		return fmt.Errorf("purchase with crypto invoice id %s not found", utils.MaskHalfInt64(purchaseId))
	}
	if purchase.Status == database.PurchaseStatusPaid {
		return nil // idempotent: already fulfilled (e.g. Stripe webhook retry)
	}

	customer, err := s.customerRepository.FindById(ctx, purchase.CustomerID)
	if err != nil {
		return err
	}
	if customer == nil {
		return fmt.Errorf("customer %s not found", utils.MaskHalfInt64(purchase.CustomerID))
	}

	oldPlan := customer.Plan
	oldExpireAt := customer.ExpireAt

	if messageId, b := s.cache.Get(purchase.ID); b {
		_, err = s.telegramBot.DeleteMessage(ctx, &bot.DeleteMessageParams{
			ChatID:    customer.TelegramID,
			MessageID: messageId,
		})
		if err != nil {
			slog.Error("Error deleting message", "error", err)
		}
	}

	previousLinkWasEmpty := customer.SubscriptionLink == nil || (customer.SubscriptionLink != nil && *customer.SubscriptionLink == "")

	// Determine plan from context (default to lite)
	plan, _ := ctx.Value(ctxkeys.Plan).(string)
	if plan == "" {
		plan = "lite"
	}

	days := purchase.Month * config.DaysInMonth()
	ctxWithPlan := context.WithValue(ctx, ctxkeys.Plan, plan)

	var user *remapi.User

	// If this is an upgrade to premium, reset expiry instead of stacking remaining time.
	if oldPlan != "premium" && plan == "premium" {
		user, err = s.remnawaveClient.SetUserSubscriptionExactByTelegramId(ctxWithPlan, customer.TelegramID, config.TrafficLimit(), days)
		if err != nil {
			return err
		}
		slog.Info("remnawave user upgraded to premium with exact window",
			"customer_id", utils.MaskHalfInt64(customer.ID),
			"telegram_id", utils.MaskHalfInt64(customer.TelegramID),
			"months", purchase.Month,
		)
	} else {
		user, err = s.remnawaveClient.CreateOrUpdateUser(ctxWithPlan, customer.ID, customer.TelegramID, config.TrafficLimit(), days, false)
		if err != nil {
			return err
		}
		slog.Info("remnawave user updated for purchase",
			"customer_id", utils.MaskHalfInt64(customer.ID),
			"telegram_id", utils.MaskHalfInt64(customer.TelegramID),
			"months", purchase.Month,
		)
	}

	err = s.purchaseRepository.MarkAsPaid(ctx, purchase.ID)
	if err != nil {
		return err
	}

	customerFilesToUpdate := map[string]interface{}{
		"subscription_link": user.SubscriptionUrl,
		"expire_at":         user.ExpireAt,
		"plan":              plan,
	}

	err = s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate)
	if err != nil {
		return err
	}

	// Plan change notification (upgrade/renewal)
	if !user.ExpireAt.IsZero() {
		if notifyErr := s.notifyPlanChange(ctx, customer, oldPlan, oldExpireAt, plan, user.ExpireAt); notifyErr != nil {
			slog.Error("error sending plan change notification", "error", notifyErr, "customer_id", utils.MaskHalfInt64(customer.ID))
		}
	}

	// If this is the first time the user receives a subscription link, send them a one-time info message.
	if previousLinkWasEmpty && user.SubscriptionUrl != "" {
		_, sendErr := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    customer.TelegramID,
			ParseMode: models.ParseModeHTML,
			Text:      s.translation.GetText(customer.Language, "unique_link_info"),
		})
		if sendErr != nil {
			slog.Error("error sending unique link info message", "error", sendErr, "telegram_id", utils.MaskHalfInt64(customer.TelegramID))
		}
	}

	// Award diamonds based on plan and period
	baseMonthly := 0
	switch plan {
	case "premium":
		baseMonthly = 10
	default:
		baseMonthly = 5
	}
	months := purchase.Month
	deltaDiamonds := baseMonthly * months
	if months == 12 {
		// 20% boost for yearly
		deltaDiamonds = int(float64(deltaDiamonds) * 1.2)
	}
	if deltaDiamonds > 0 {
		// Refresh customer to avoid using stale diamonds value
		freshCustomer, err := s.customerRepository.FindById(ctx, customer.ID)
		if err != nil {
			return err
		}
		currentDiamonds := 0
		if freshCustomer != nil {
			currentDiamonds = freshCustomer.Diamonds
		}
		if err := s.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
			"diamonds": currentDiamonds + deltaDiamonds,
		}); err != nil {
			return err
		}
		slog.Info("diamonds granted", "customer_id", utils.MaskHalfInt64(customer.ID), "plan", plan, "months", months, "diamonds", deltaDiamonds)
	}

	_, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: customer.TelegramID,
		Text:   s.translation.GetText(customer.Language, "subscription_activated"),
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: s.createConnectKeyboard(customer),
		},
	})
	if err != nil {
		return err
	}

	slog.Info("checking conditions for Moynalog receipt", "invoice_type", purchase.InvoiceType, "moynalog_client", s.moynalogClient != nil)
	if purchase.InvoiceType == database.InvoiceTypeYookasa && s.moynalogClient != nil {
		slog.Info("attempting to send receipt to Moynalog", "purchase_id", utils.MaskHalfInt64(purchase.ID), "amount", purchase.Amount, "month", purchase.Month)
		go func() {
			moynalogCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			err := s.sendReceiptToMoynalog(moynalogCtx, purchase)
			if err != nil {
				slog.Error("error sending receipt to Moynalog", "error", err, "purchase_id", utils.MaskHalfInt64(purchase.ID))
				_, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: config.GetAdminTelegramId(),
					Text:   "Ошибка при отправке чека в Мой налог. Проверьте логи.",
				})
				if err != nil {
					slog.Error("error while sending moy nalog error message", "error", err, "purchase_id", utils.MaskHalfInt64(purchase.ID))
				}
			} else {
				slog.Info("successfully sent receipt to Moynalog", "purchase_id", utils.MaskHalfInt64(purchase.ID))
			}
		}()
	} else {
		if purchase.InvoiceType != database.InvoiceTypeYookasa {
			slog.Info("not sending receipt to Moynalog - not a Yookasa invoice", "invoice_type", purchase.InvoiceType, "purchase_id", utils.MaskHalfInt64(purchase.ID))
		} else if s.moynalogClient == nil {
			slog.Error("not sending receipt to Moynalog - client is nil", "purchase_id", utils.MaskHalfInt64(purchase.ID))
		}
	}

	ctxReferee := context.Background()
	referralRecord, err := s.referralRepository.FindByReferee(ctxReferee, customer.TelegramID)
	if err != nil {
		return err
	}
	if referralRecord != nil && !referralRecord.BonusGranted {
		const referralBonusDiamonds = 5

		// Award +5 diamonds to the paying user (referee)
		refereeFresh, err := s.customerRepository.FindById(ctxReferee, customer.ID)
		if err != nil {
			return err
		}
		refereeDiamonds := 0
		if refereeFresh != nil {
			refereeDiamonds = refereeFresh.Diamonds
		}
		if err := s.customerRepository.UpdateFields(ctxReferee, customer.ID, map[string]interface{}{
			"diamonds": refereeDiamonds + referralBonusDiamonds,
		}); err != nil {
			return err
		}

		// Award +5 diamonds to the referrer
		referrerCustomer, err := s.customerRepository.FindByTelegramId(ctxReferee, referralRecord.ReferrerID)
		if err != nil {
			return err
		}
		if referrerCustomer != nil {
			referrerFresh, err := s.customerRepository.FindById(ctxReferee, referrerCustomer.ID)
			if err != nil {
				return err
			}
			referrerDiamonds := 0
			if referrerFresh != nil {
				referrerDiamonds = referrerFresh.Diamonds
			}
			if err := s.customerRepository.UpdateFields(ctxReferee, referrerCustomer.ID, map[string]interface{}{
				"diamonds": referrerDiamonds + referralBonusDiamonds,
			}); err != nil {
				return err
			}

			// Extend referrer's subscription as before
			referrerUser, err := s.remnawaveClient.CreateOrUpdateUser(ctxReferee, referrerCustomer.ID, referrerCustomer.TelegramID, config.TrafficLimit(), config.GetReferralDays(), false)
			if err != nil {
				return err
			}
			referrerUserFilesToUpdate := map[string]interface{}{
				"subscription_link": referrerUser.GetSubscriptionUrl(),
				"expire_at":         referrerUser.GetExpireAt(),
			}
			if err := s.customerRepository.UpdateFields(ctxReferee, referrerCustomer.ID, referrerUserFilesToUpdate); err != nil {
				return err
			}

			// Mark referral as processed for bonus (first paid purchase)
			if err := s.referralRepository.MarkBonusGranted(ctxReferee, referralRecord.ID); err != nil {
				return err
			}

			slog.Info("Granted referral bonus", "customer_id", utils.MaskHalfInt64(referrerCustomer.ID))

			// Notify referrer about diamonds
			_, err = s.telegramBot.SendMessage(ctxReferee, &bot.SendMessageParams{
				ChatID:    referrerCustomer.TelegramID,
				ParseMode: models.ParseModeHTML,
				Text:      s.translation.GetText(referrerCustomer.Language, "referral_diamonds_granted"),
				ReplyMarkup: models.InlineKeyboardMarkup{
					InlineKeyboard: s.createConnectKeyboard(referrerCustomer),
				},
			})
			if err != nil {
				return err
			}
		}
	}

	slog.Info("purchase processed", "purchase_id", utils.MaskHalfInt64(purchase.ID), "type", purchase.InvoiceType, "customer_id", utils.MaskHalfInt64(customer.ID))

	return nil
}

// AdminSetSubscription allows an admin to directly set a customer's plan and duration
// without creating a purchase. It updates the Remnawave user and keeps subscription
// link / expiry in sync with the VPN backend, then updates the customer record.
func (s PaymentService) AdminSetSubscription(ctx context.Context, telegramId int64, plan string, months int) (*database.Customer, error) {
	if months <= 0 {
		return nil, fmt.Errorf("invalid months value: %d", months)
	}
	if plan == "" {
		plan = "lite"
	}

	customer, err := s.customerRepository.FindByTelegramId(ctx, telegramId)
	if err != nil {
		return nil, err
	}
	if customer == nil {
		return nil, ErrCustomerNotFound
	}

	oldPlan := customer.Plan
	oldExpireAt := customer.ExpireAt

	previousLinkWasEmpty := customer.SubscriptionLink == nil || (customer.SubscriptionLink != nil && *customer.SubscriptionLink == "")

	days := months * config.DaysInMonth()

	ctxWithPlan := context.WithValue(ctx, ctxkeys.Plan, plan)

	var user *remapi.User

	// For admin upgrades to premium, also reset the window exactly.
	if oldPlan != "premium" && plan == "premium" {
		user, err = s.remnawaveClient.SetUserSubscriptionExactByTelegramId(ctxWithPlan, customer.TelegramID, config.TrafficLimit(), days)
		if err != nil {
			return nil, err
		}
	} else {
		user, err = s.remnawaveClient.CreateOrUpdateUser(ctxWithPlan, customer.ID, customer.TelegramID, config.TrafficLimit(), days, false)
		if err != nil {
			return nil, err
		}
	}

	customerFieldsToUpdate := map[string]interface{}{
		"subscription_link": user.SubscriptionUrl,
		"expire_at":         user.ExpireAt,
		"plan":              plan,
	}

	if err := s.customerRepository.UpdateFields(ctx, customer.ID, customerFieldsToUpdate); err != nil {
		return nil, err
	}

	// Refresh customer to return up-to-date data.
	updatedCustomer, err := s.customerRepository.FindById(ctx, customer.ID)
	if err != nil {
		return nil, err
	}

	// Plan change notification (upgrade/renewal)
	if !user.ExpireAt.IsZero() {
		if notifyErr := s.notifyPlanChange(ctx, customer, oldPlan, oldExpireAt, plan, user.ExpireAt); notifyErr != nil {
			slog.Error("error sending plan change notification (admin)", "error", notifyErr, "customer_id", utils.MaskHalfInt64(customer.ID))
		}
	}

	// If this is the first time the user receives a subscription link, send them a one-time info message.
	if previousLinkWasEmpty && user.SubscriptionUrl != "" {
		_, sendErr := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    customer.TelegramID,
			ParseMode: models.ParseModeHTML,
			Text:      s.translation.GetText(customer.Language, "unique_link_info"),
		})
		if sendErr != nil {
			slog.Error("error sending unique link info message (admin)", "error", sendErr, "telegram_id", utils.MaskHalfInt64(customer.TelegramID))
		}
	}

	return updatedCustomer, nil
}

// AdminRemoveSubscription allows an admin to immediately revoke a customer's subscription.
// It updates the Remnawave user to have zero traffic limit and marks the customer as free-tier
// with an already expired subscription in the local database.
func (s PaymentService) AdminRemoveSubscription(ctx context.Context, telegramId int64) (*database.Customer, error) {
	customer, err := s.customerRepository.FindByTelegramId(ctx, telegramId)
	if err != nil {
		return nil, err
	}
	if customer == nil {
		return nil, ErrCustomerNotFound
	}

	// Revoke subscription in Remnawave so existing configs/links stop working.
	if err := s.remnawaveClient.RevokeUserSubscriptionByTelegramId(ctx, customer.TelegramID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	updates := map[string]interface{}{
		"plan":      "free",
		"expire_at": now,
	}

	if err := s.customerRepository.UpdateFields(ctx, customer.ID, updates); err != nil {
		return nil, err
	}

	return s.customerRepository.FindById(ctx, customer.ID)
}

func (s PaymentService) notifyPlanChange(ctx context.Context, customer *database.Customer, oldPlan string, oldExpireAt *time.Time, newPlan string, newExpireAt time.Time) error {
	oldRank := domain.PlanRank(oldPlan)
	newRank := domain.PlanRank(newPlan)

	if newRank == 0 {
		return nil
	}

	var key string
	switch {
	case oldRank == 0 && newRank > 0:
		key = "plan_upgraded_message"
	case newRank > oldRank:
		key = "plan_upgraded_message"
	case newRank == oldRank:
		if oldExpireAt != nil && newExpireAt.After(*oldExpireAt) {
			key = "plan_renewed_message"
		}
	}

	if key == "" {
		return nil
	}

	expireDate := newExpireAt.Format("02.01.2006")
	var text string
	switch key {
	case "plan_upgraded_message":
		text = fmt.Sprintf(s.translation.GetText(customer.Language, key), newPlan, expireDate)
	case "plan_renewed_message":
		text = fmt.Sprintf(s.translation.GetText(customer.Language, key), newPlan, expireDate)
	}

	if text == "" {
		return nil
	}

	_, err := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    customer.TelegramID,
		ParseMode: models.ParseModeHTML,
		Text:      text,
	})
	return err
}

func (s PaymentService) createConnectKeyboard(customer *database.Customer) [][]models.InlineKeyboardButton {
	var inlineCustomerKeyboard [][]models.InlineKeyboardButton

	inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
		{Text: s.translation.GetText(customer.Language, "my_links_button"), CallbackData: "my_links"},
	})
	inlineCustomerKeyboard = append(inlineCustomerKeyboard, []models.InlineKeyboardButton{
		{Text: s.translation.GetText(customer.Language, "back_button"), CallbackData: "start"},
	})
	return inlineCustomerKeyboard
}

func (s PaymentService) CreatePurchase(ctx context.Context, amount float64, months int, customer *database.Customer, invoiceType database.InvoiceType) (url string, purchaseId int64, err error) {
	switch invoiceType {
	case database.InvoiceTypeCrypto:
		return s.createCryptoInvoice(ctx, amount, months, customer)
	case database.InvoiceTypeYookasa:
		return s.createYookasaInvoice(ctx, amount, months, customer)
	case database.InvoiceTypeTelegram:
		return s.createTelegramInvoice(ctx, amount, months, customer)
	case database.InvoiceTypeTribute:
		return s.createTributeInvoice(ctx, amount, months, customer)
	case database.InvoiceTypeStripe:
		return s.createStripeInvoice(ctx, amount, months, customer)
	default:
		return "", 0, fmt.Errorf("unknown invoice type: %s", invoiceType)
	}
}

var ErrCustomerNotFound = errors.New("customer not found")

func (s PaymentService) CancelTributePurchase(ctx context.Context, telegramId int64) error {
	slog.Info("Canceling tribute purchase", "telegram_id", utils.MaskHalfInt64(telegramId))
	customer, err := s.customerRepository.FindByTelegramId(ctx, telegramId)
	if err != nil {
		return err
	}
	if customer == nil {
		return ErrCustomerNotFound
	}
	tributePurchase, err := s.purchaseRepository.FindByCustomerIDAndInvoiceTypeLast(ctx, customer.ID, database.InvoiceTypeTribute)
	if err != nil {
		return err
	}
	if tributePurchase == nil {
		return errors.New("tribute purchase not found")
	}
	expireAt, err := s.remnawaveClient.DecreaseSubscription(ctx, telegramId, config.TrafficLimit(), -tributePurchase.Month*config.DaysInMonth())
	if err != nil {
		return err
	}

	if err := s.customerRepository.UpdateFields(ctx, customer.ID, map[string]interface{}{
		"expire_at": expireAt,
	}); err != nil {
		return err
	}

	if err := s.purchaseRepository.UpdateFields(ctx, tributePurchase.ID, map[string]interface{}{
		"status": database.PurchaseStatusCancel,
	}); err != nil {
		return err
	}
	_, err = s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    telegramId,
		ParseMode: models.ParseModeHTML,
		Text:      s.translation.GetText(customer.Language, "tribute_cancelled"),
	})
	if err != nil {
		slog.Error("Error sending message about tribute cancelled", "error", err, "telegram_id", utils.MaskHalfInt64(telegramId))
	}
	slog.Info("Canceled tribute purchase", "purchase_id", utils.MaskHalfInt64(tributePurchase.ID), "telegram_id", utils.MaskHalfInt64(telegramId))
	return nil
}

func (s PaymentService) createCryptoInvoice(ctx context.Context, amount float64, months int, customer *database.Customer) (url string, purchaseId int64, err error) {
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType: database.InvoiceTypeCrypto,
		Status:      database.PurchaseStatusNew,
		Amount:      amount,
		Currency:    "RUB",
		CustomerID:  customer.ID,
		Month:       months,
	})
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	plan, _ := ctx.Value(ctxkeys.Plan).(string)
	invoice, err := s.cryptoPayClient.CreateInvoice(&cryptopay.InvoiceRequest{
		CurrencyType:   "fiat",
		Fiat:           "RUB",
		Amount:         fmt.Sprintf("%d", int(amount)),
		AcceptedAssets: "USDT",
		Payload:        fmt.Sprintf("purchaseId=%d&username=%s&plan=%s", purchaseId, ctx.Value(ctxkeys.Username), plan),
		Description:    fmt.Sprintf("Subscription on %d month", months),
		PaidBtnName:    "callback",
		PaidBtnUrl:     config.BotURL(),
	})
	if err != nil {
		slog.Error("Error creating invoice", "error", err)
		return "", 0, err
	}

	updates := map[string]interface{}{
		"crypto_invoice_url": invoice.BotInvoiceUrl,
		"crypto_invoice_id":  invoice.InvoiceID,
		"status":             database.PurchaseStatusPending,
	}

	err = s.purchaseRepository.UpdateFields(ctx, purchaseId, updates)
	if err != nil {
		slog.Error("Error updating purchase", "error", err)
		return "", 0, err
	}

	return invoice.BotInvoiceUrl, purchaseId, nil
}

func (s PaymentService) createYookasaInvoice(ctx context.Context, amount float64, months int, customer *database.Customer) (url string, purchaseId int64, err error) {
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType: database.InvoiceTypeYookasa,
		Status:      database.PurchaseStatusNew,
		Amount:      amount,
		Currency:    "RUB",
		CustomerID:  customer.ID,
		Month:       months,
	})
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	invoice, err := s.yookasaClient.CreateInvoice(ctx, int(amount), months, customer.ID, purchaseId)
	if err != nil {
		slog.Error("Error creating invoice", "error", err)
		return "", 0, err
	}

	updates := map[string]interface{}{
		"yookasa_url": invoice.Confirmation.ConfirmationURL,
		"yookasa_id":  invoice.ID,
		"status":      database.PurchaseStatusPending,
	}

	err = s.purchaseRepository.UpdateFields(ctx, purchaseId, updates)
	if err != nil {
		slog.Error("Error updating purchase", "error", err)
		return "", 0, err
	}

	return invoice.Confirmation.ConfirmationURL, purchaseId, nil
}

// createStripeInvoice creates a pending Stripe purchase and a Checkout Session; returns the checkout URL.
func (s PaymentService) createStripeInvoice(ctx context.Context, amount float64, months int, customer *database.Customer) (url string, purchaseId int64, err error) {
	if s.stripeClient == nil {
		return "", 0, errors.New("stripe client not configured")
	}
	amountCents := int(amount)
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType: database.InvoiceTypeStripe,
		Status:      database.PurchaseStatusNew,
		Amount:      float64(amountCents) / 100,
		Currency:    "USD",
		CustomerID:  customer.ID,
		Month:       months,
	})
	if err != nil {
		slog.Error("Error creating purchase for Stripe", "error", err)
		return "", 0, err
	}

	sessionID, checkoutURL, err := s.stripeClient.CreateCheckoutSession(ctx, amountCents, months, purchaseId, config.StripeSuccessURL(), config.StripeCancelURL())
	if err != nil {
		slog.Error("Error creating Stripe checkout session", "error", err)
		return "", 0, err
	}

	updates := map[string]interface{}{
		"stripe_session_id":   sessionID,
		"stripe_checkout_url": checkoutURL,
		"status":             database.PurchaseStatusPending,
	}
	if err = s.purchaseRepository.UpdateFields(ctx, purchaseId, updates); err != nil {
		slog.Error("Error updating purchase with Stripe session", "error", err)
		return "", 0, err
	}
	return checkoutURL, purchaseId, nil
}

func (s PaymentService) createTelegramInvoice(ctx context.Context, amount float64, months int, customer *database.Customer) (url string, purchaseId int64, err error) {
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType: database.InvoiceTypeTelegram,
		Status:      database.PurchaseStatusNew,
		Amount:      amount,
		Currency:    "STARS",
		CustomerID:  customer.ID,
		Month:       months,
	})
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	plan, _ := ctx.Value(ctxkeys.Plan).(string)
	invoiceUrl, err := s.telegramBot.CreateInvoiceLink(ctx, &bot.CreateInvoiceLinkParams{
		Title:    s.translation.GetText(customer.Language, "invoice_title"),
		Currency: "XTR",
		Prices: []models.LabeledPrice{
			{
				Label:  s.translation.GetText(customer.Language, "invoice_label"),
				Amount: int(amount),
			},
		},
		Description: s.translation.GetText(customer.Language, "invoice_description"),
		Payload:     fmt.Sprintf("%d&%s&%s", purchaseId, ctx.Value(ctxkeys.Username), plan),
	})

	updates := map[string]interface{}{
		"status": database.PurchaseStatusPending,
	}

	err = s.purchaseRepository.UpdateFields(ctx, purchaseId, updates)
	if err != nil {
		slog.Error("Error updating purchase", "error", err)
		return "", 0, err
	}

	return invoiceUrl, purchaseId, nil
}

func (s PaymentService) ActivateTrial(ctx context.Context, telegramId int64) (string, error) {
	if config.TrialDays() == 0 {
		return "", nil
	}
	customer, err := s.customerRepository.FindByTelegramId(ctx, telegramId)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return "", err
	}
	if customer == nil {
		return "", fmt.Errorf("customer %d not found", telegramId)
	}
	previousLinkWasEmpty := customer.SubscriptionLink == nil || (customer.SubscriptionLink != nil && *customer.SubscriptionLink == "")

	user, err := s.remnawaveClient.CreateOrUpdateUser(ctx, customer.ID, telegramId, config.TrialTrafficLimit(), config.TrialDays(), true)
	if err != nil {
		slog.Error("Error creating user", "error", err)
		return "", err
	}

	slog.Info("remnawave user updated for trial",
		"customer_id", utils.MaskHalfInt64(customer.ID),
		"telegram_id", utils.MaskHalfInt64(telegramId),
	)

	customerFilesToUpdate := map[string]interface{}{
		"subscription_link": user.GetSubscriptionUrl(),
		"expire_at":         user.GetExpireAt(),
		"diamonds":          customer.Diamonds + 1,
		"plan":              "free",
	}

	err = s.customerRepository.UpdateFields(ctx, customer.ID, customerFilesToUpdate)
	if err != nil {
		return "", err
	}

	// If this is the first time the user receives a subscription link, send them a one-time info message.
	if previousLinkWasEmpty && user.GetSubscriptionUrl() != "" {
		_, sendErr := s.telegramBot.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    telegramId,
			ParseMode: models.ParseModeHTML,
			Text:      s.translation.GetText(customer.Language, "unique_link_info"),
		})
		if sendErr != nil {
			slog.Error("error sending unique link info message (trial)", "error", sendErr, "telegram_id", utils.MaskHalfInt64(telegramId))
		}
	}

	return user.GetSubscriptionUrl(), nil

}

func (s PaymentService) CancelYookassaPayment(purchaseId int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	purchase, err := s.purchaseRepository.FindById(ctx, purchaseId)
	if err != nil {
		return err
	}
	if purchase == nil {
		return fmt.Errorf("purchase with crypto invoice id %s not found", utils.MaskHalfInt64(purchaseId))
	}

	purchaseFieldsToUpdate := map[string]interface{}{
		"status": database.PurchaseStatusCancel,
	}

	err = s.purchaseRepository.UpdateFields(ctx, purchaseId, purchaseFieldsToUpdate)
	if err != nil {
		return err
	}

	return nil
}

func (s PaymentService) createTributeInvoice(ctx context.Context, amount float64, months int, customer *database.Customer) (url string, purchaseId int64, err error) {
	purchaseId, err = s.purchaseRepository.Create(ctx, &database.Purchase{
		InvoiceType: database.InvoiceTypeTribute,
		Status:      database.PurchaseStatusPending,
		Amount:      amount,
		Currency:    "RUB",
		CustomerID:  customer.ID,
		Month:       months,
	})
	if err != nil {
		slog.Error("Error creating purchase", "error", err)
		return "", 0, err
	}

	return "", purchaseId, nil
}

func (s PaymentService) sendReceiptToMoynalog(ctx context.Context, purchase *database.Purchase) error {
	if s.moynalogClient == nil {
		return fmt.Errorf("moynalog client not initialized")
	}

	var monthString string
	switch purchase.Month {
	case 1:
		monthString = "месяц"
	case 3, 4:
		monthString = "месяца"
	default:
		monthString = "месяцев"
	}
	comment := fmt.Sprintf("Подписка на %d %s", purchase.Month, monthString)
	amount := purchase.Amount

	_, err := s.moynalogClient.CreateIncome(ctx, amount, comment)
	if err != nil {
		return fmt.Errorf("failed to create income in Moynalog: %w", err)
	}

	slog.Info("Receipt sent to Moynalog", "purchase_id", utils.MaskHalfInt64(purchase.ID), "amount", amount, "comment", comment)
	return nil
}
