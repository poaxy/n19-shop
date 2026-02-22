package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

func (h Handler) BuyCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message
	langCode := update.CallbackQuery.From.LanguageCode

	var priceButtons []models.InlineKeyboardButton

	if config.Price1() > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetText(langCode, "month_1"),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 1, config.Price1()),
		})
	}

	if config.Price3() > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetText(langCode, "month_3"),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 3, config.Price3()),
		})
	}

	if config.Price6() > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetText(langCode, "month_6"),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 6, config.Price6()),
		})
	}

	if config.Price12() > 0 {
		priceButtons = append(priceButtons, models.InlineKeyboardButton{
			Text:         h.translation.GetText(langCode, "month_12"),
			CallbackData: fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, 12, config.Price12()),
		})
	}

	keyboard := [][]models.InlineKeyboardButton{}

	if len(priceButtons) == 4 {
		keyboard = append(keyboard, priceButtons[:2])
		keyboard = append(keyboard, priceButtons[2:])
	} else if len(priceButtons) > 0 {
		keyboard = append(keyboard, priceButtons)
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
	})

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
		Text: h.translation.GetText(langCode, "pricing_info"),
	})

	if err != nil {
		slog.Error("Error editing message for buy", "error", err)
	}
}

// hasDirectPaymentMethods returns true if any of Stripe or Crypto is enabled (direct payment options shown in UI).
func hasDirectPaymentMethods() bool {
	return config.IsStripeEnabled() || config.IsCryptoPayEnabled()
}

// shouldShowStarsButton returns true if the Stars payment option should be shown (Stars enabled and, when required, user has a prior paid purchase).
func (h Handler) shouldShowStarsButton(ctx context.Context, chatID int64) bool {
	if !config.IsTelegramStarsEnabled() {
		return false
	}
	if !config.RequirePaidPurchaseForStars() {
		return true
	}
	customer, err := h.customerRepository.FindByTelegramId(ctx, chatID)
	if err != nil || customer == nil {
		return false
	}
	paidPurchase, err := h.purchaseRepository.FindSuccessfulPaidPurchaseByCustomer(ctx, customer.ID)
	if err != nil {
		return false
	}
	return paidPurchase != nil
}

// shouldShowPaymentMethodChoice returns true when we show "Telegram Stars" vs "Direct payment" first.
func (h Handler) shouldShowPaymentMethodChoice(ctx context.Context, chatID int64) bool {
	if !config.IsTelegramStarsEnabled() || !hasDirectPaymentMethods() {
		return false
	}
	// Only use two-step when user can see Stars; otherwise show direct methods only.
	return h.shouldShowStarsButton(ctx, chatID)
}

func (h Handler) SellCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	langCode := update.CallbackQuery.From.LanguageCode
	month := callbackQuery["month"]
	amount := callbackQuery["amount"]

	var keyboard [][]models.InlineKeyboardButton

	// When both Stars and direct methods exist, show "Telegram Stars" vs "Direct payment" first.
	if h.shouldShowPaymentMethodChoice(ctx, callback.Chat.ID) {
		shouldShowStars := h.shouldShowStarsButton(ctx, callback.Chat.ID)
		if shouldShowStars {
			keyboard = append(keyboard, []models.InlineKeyboardButton{
				{Text: h.translation.GetText(langCode, "stars_button"), CallbackData: fmt.Sprintf("%s?month=%s&invoiceType=%s&amount=%s", CallbackPayment, month, database.InvoiceTypeTelegram, amount)},
			})
		}
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "direct_payment_button"), CallbackData: fmt.Sprintf("%s?month=%s&amount=%s", CallbackDirect, month, amount)},
		})
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackBuy},
		})

		_, err := b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
			ChatID:    callback.Chat.ID,
			MessageID: callback.ID,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: keyboard,
			},
		})
		if err != nil {
			slog.Error("Error editing message for payment method choice", "error", err)
		}
		return
	}

	// Otherwise show payment methods directly (only direct methods, or only Stars when no direct).
	h.appendPaymentMethodButtons(ctx, &keyboard, langCode, month, amount, callback.Chat.ID)
	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackBuy},
	})

	_, err := b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: keyboard},
	})
	if err != nil {
		slog.Error("Error editing message for payment methods", "error", err)
	}
}

// appendPaymentMethodButtons appends enabled payment methods (Crypto, Stripe, Stars only) to keyboard.
func (h Handler) appendPaymentMethodButtons(ctx context.Context, keyboard *[][]models.InlineKeyboardButton, langCode, month, amount string, chatID int64) {
	if config.IsCryptoPayEnabled() {
		*keyboard = append(*keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "crypto_button"), CallbackData: fmt.Sprintf("%s?month=%s&invoiceType=%s&amount=%s", CallbackPayment, month, database.InvoiceTypeCrypto, amount)},
		})
	}

	if config.IsStripeEnabled() {
		*keyboard = append(*keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "stripe_button"), CallbackData: fmt.Sprintf("%s?month=%s&invoiceType=%s&amount=%s", CallbackPayment, month, database.InvoiceTypeStripe, amount)},
		})
	}

	if config.IsTelegramStarsEnabled() && h.shouldShowStarsButton(ctx, chatID) {
		*keyboard = append(*keyboard, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "stars_button"), CallbackData: fmt.Sprintf("%s?month=%s&invoiceType=%s&amount=%s", CallbackPayment, month, database.InvoiceTypeTelegram, amount)},
		})
	}
}

// buildDirectPaymentKeyboard returns inline keyboard rows for direct payment methods only (Stripe, Crypto).
func (h Handler) buildDirectPaymentKeyboard(langCode, month, amount string) [][]models.InlineKeyboardButton {
	var rows [][]models.InlineKeyboardButton
	if config.IsStripeEnabled() {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "stripe_button"), CallbackData: fmt.Sprintf("%s?month=%s&invoiceType=%s&amount=%s", CallbackPayment, month, database.InvoiceTypeStripe, amount)},
		})
	}
	if config.IsCryptoPayEnabled() {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: h.translation.GetText(langCode, "crypto_button"), CallbackData: fmt.Sprintf("%s?month=%s&invoiceType=%s&amount=%s", CallbackPayment, month, database.InvoiceTypeCrypto, amount)},
		})
	}
	return rows
}

// DirectPaymentCallbackHandler shows direct payment options (Crypto, Card, Tribute) and Back when user chose "Direct payment".
func (h Handler) DirectPaymentCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	langCode := update.CallbackQuery.From.LanguageCode
	month := callbackQuery["month"]
	amount := callbackQuery["amount"]

	keyboard := h.buildDirectPaymentKeyboard(langCode, month, amount)
	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: fmt.Sprintf("%s?month=%s&amount=%s", CallbackSell, month, amount)},
	})

	_, err := b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ReplyMarkup: models.InlineKeyboardMarkup{InlineKeyboard: keyboard},
	})
	if err != nil {
		slog.Error("Error editing message for direct payment", "error", err)
	}
}

func (h Handler) PaymentCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	callback := update.CallbackQuery.Message.Message
	callbackQuery := parseCallbackData(update.CallbackQuery.Data)
	month, err := strconv.Atoi(callbackQuery["month"])
	if err != nil {
		slog.Error("Error getting month from query", "error", err)
		return
	}

	invoiceType := database.InvoiceType(callbackQuery["invoiceType"])

	var price int
	switch invoiceType {
	case database.InvoiceTypeTelegram:
		price = config.StarsPrice(month)
	case database.InvoiceTypeStripe:
		price = config.StripePrice(month)
	default:
		price = config.Price(month)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	customer, err := h.customerRepository.FindByTelegramId(ctx, callback.Chat.ID)
	if err != nil {
		slog.Error("Error finding customer", "error", err)
		return
	}
	if customer == nil {
		slog.Error("Customer not found for payment", "chatID", callback.Chat.ID)
		return
	}

	ctxWithUsername := context.WithValue(ctx, "username", update.CallbackQuery.From.Username)
	paymentURL, purchaseID, err := h.paymentService.CreatePurchase(ctxWithUsername, float64(price), month, customer, invoiceType)
	if err != nil {
		slog.Error("Error creating payment", "error", err)
		return
	}

	langCode := update.CallbackQuery.From.LanguageCode
	backCallback := fmt.Sprintf("%s?month=%d&amount=%d", CallbackSell, month, price)
	if invoiceType != database.InvoiceTypeTelegram && hasDirectPaymentMethods() && config.IsTelegramStarsEnabled() {
		backCallback = fmt.Sprintf("%s?month=%d&amount=%d", CallbackDirect, month, price)
	}
	message, err := b.EditMessageReplyMarkup(ctx, &bot.EditMessageReplyMarkupParams{
		ChatID:    callback.Chat.ID,
		MessageID: callback.ID,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: h.translation.GetText(langCode, "pay_button"), URL: paymentURL},
					{Text: h.translation.GetText(langCode, "back_button"), CallbackData: backCallback},
				},
			},
		},
	})
	if err != nil {
		slog.Error("Error editing message for payment", "error", err)
		return
	}
	h.cache.Set(purchaseID, message.ID)
}

func (h Handler) PreCheckoutCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	_, err := b.AnswerPreCheckoutQuery(ctx, &bot.AnswerPreCheckoutQueryParams{
		PreCheckoutQueryID: update.PreCheckoutQuery.ID,
		OK:                 true,
	})
	if err != nil {
		slog.Error("Error sending answer pre checkout query", "error", err)
	}
}

func (h Handler) SuccessPaymentHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	payload := strings.Split(update.Message.SuccessfulPayment.InvoicePayload, "&")
	if len(payload) < 2 {
		slog.Error("Invalid payment payload: missing parts", "payload", update.Message.SuccessfulPayment.InvoicePayload)
		return
	}
	purchaseID, err := strconv.ParseInt(payload[0], 10, 64)
	if err != nil {
		slog.Error("Error parsing purchase id from payload", "error", err, "payload", payload[0])
		return
	}
	username := payload[1]
	ctxWithUsername := context.WithValue(ctx, "username", username)
	if err := h.paymentService.ProcessPurchaseById(ctxWithUsername, purchaseID); err != nil {
		slog.Error("Error processing purchase", "error", err, "purchaseID", purchaseID)
	}
}

// parseCallbackData parses callback data in the form "prefix?key1=val1&key2=val2" into a map.
func parseCallbackData(data string) map[string]string {
	result := make(map[string]string)
	parts := strings.Split(data, "?")
	if len(parts) < 2 {
		return result
	}
	for _, param := range strings.Split(parts[1], "&") {
		kv := strings.SplitN(param, "=", 2)
		if len(kv) == 2 {
			result[kv[0]] = kv[1]
		}
	}
	return result
}
