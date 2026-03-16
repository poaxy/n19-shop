package handler

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/admin"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/pricing"
	"remnawave-tg-shop-bot/internal/translation"
)

func (h Handler) AdminToolsCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_tools_menu_title"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "admin_tools_reset_diamonds_button"), CallbackData: CallbackAdminToolsResetDiamonds}},
				{{Text: h.translation.GetText(langCode, "admin_tools_sync_button"), CallbackData: CallbackAdminToolsSync}},
				{{Text: h.translation.GetText(langCode, "admin_tools_stats_button"), CallbackData: CallbackAdminToolsStats}},
				{{Text: h.translation.GetText(langCode, "admin_tools_download_db_button"), CallbackData: CallbackAdminToolsDownloadDB}},
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			},
		},
	})
	if err != nil {
		slog.Error("error showing admin tools menu", "error", err)
	}
}

func (h Handler) AdminToolsResetDiamondsCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_tools_reset_diamonds_prompt"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "admin_tools_reset_diamonds_confirm_button"), CallbackData: CallbackAdminToolsResetDiamondsConfirm}},
				{{Text: h.translation.GetText(langCode, "admin_tools_reset_diamonds_cancel_button"), CallbackData: CallbackAdminTools}},
			},
		},
	})
	if err != nil {
		slog.Error("error showing reset diamonds confirmation", "error", err)
	}
}

func (h Handler) AdminToolsResetDiamondsConfirmCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	if err := h.customerRepository.ResetAllDiamonds(ctx); err != nil {
		slog.Error("error resetting all diamonds", "error", err)
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    callbackMessage.Chat.ID,
			MessageID: callbackMessage.ID,
			Text:      h.translation.GetText(langCode, "admin_notify_error"),
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	slog.Info("admin reset all diamonds")

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_tools_reset_diamonds_done"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminTools}},
			},
		},
	})
	if err != nil {
		slog.Error("error sending reset diamonds result", "error", err)
	}
}

func (h Handler) AdminToolsSyncCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	slog.Info("admin triggered user sync")
	h.syncService.Sync()

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_tools_sync_done"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminTools}},
			},
		},
	})
	if err != nil {
		slog.Error("error sending sync result", "error", err)
	}
}

func (h Handler) AdminToolsStatsCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	now := time.Now().UTC()
	total, err := h.customerRepository.CountAll(ctx)
	if err != nil {
		slog.Error("error counting all users", "error", err)
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    callbackMessage.Chat.ID,
			MessageID: callbackMessage.ID,
			Text:      h.translation.GetText(langCode, "admin_notify_error"),
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	freeActive, err := h.customerRepository.CountActiveByPlan(ctx, "free", now)
	if err != nil {
		slog.Error("error counting free active users", "error", err)
	}
	liteActive, err := h.customerRepository.CountActiveByPlan(ctx, "lite", now)
	if err != nil {
		slog.Error("error counting lite active users", "error", err)
	}
	premiumActive, err := h.customerRepository.CountActiveByPlan(ctx, "premium", now)
	if err != nil {
		slog.Error("error counting premium active users", "error", err)
	}

	lines := []string{
		fmt.Sprintf(h.translation.GetText(langCode, "admin_tools_stats_line_total"), total),
		fmt.Sprintf(h.translation.GetText(langCode, "admin_tools_stats_line_free_active"), freeActive),
		fmt.Sprintf(h.translation.GetText(langCode, "admin_tools_stats_line_lite_active"), liteActive),
		fmt.Sprintf(h.translation.GetText(langCode, "admin_tools_stats_line_premium_active"), premiumActive),
	}

	text := h.translation.GetText(langCode, "admin_tools_stats_title") + "\n" + strings.Join(lines, "\n")

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminTools}},
			},
		},
	})
	if err != nil {
		slog.Error("error sending admin stats", "error", err)
	}
}

func (h Handler) AdminToolsDownloadDatabaseCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)

	// Rate limit: once per minute per admin
	const rateLimitKeyOffset int64 = 1000000000000
	cacheKey := cb.From.ID + rateLimitKeyOffset

	now := time.Now().Unix()
	if last, ok := h.cache.Get(cacheKey); ok {
		if now-int64(last) < 60 {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    cb.Message.Message.Chat.ID,
				ParseMode: models.ParseModeHTML,
				Text:      h.translation.GetText(langCode, "admin_tools_download_db_too_frequent"),
			})
			return
		}
	}
	h.cache.Set(cacheKey, int(now))

	customers, err := h.customerRepository.ListWithDiamonds(ctx)
	if err != nil {
		slog.Error("error listing customers with diamonds", "error", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    cb.Message.Message.Chat.ID,
			ParseMode: models.ParseModeHTML,
			Text:      h.translation.GetText(langCode, "admin_tools_download_db_error"),
		})
		return
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	if err := writer.Write([]string{"telegram_id", "diamonds", "plan"}); err != nil {
		slog.Error("error writing csv header", "error", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    cb.Message.Message.Chat.ID,
			ParseMode: models.ParseModeHTML,
			Text:      h.translation.GetText(langCode, "admin_tools_download_db_error"),
		})
		return
	}

	for _, c := range customers {
		record := []string{
			strconv.FormatInt(c.TelegramID, 10),
			strconv.Itoa(c.Diamonds),
			c.Plan,
		}
		if err := writer.Write(record); err != nil {
			slog.Error("error writing csv record", "error", err)
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    cb.Message.Message.Chat.ID,
				ParseMode: models.ParseModeHTML,
				Text:      h.translation.GetText(langCode, "admin_tools_download_db_error"),
			})
			return
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		slog.Error("error flushing csv writer", "error", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    cb.Message.Message.Chat.ID,
			ParseMode: models.ParseModeHTML,
			Text:      h.translation.GetText(langCode, "admin_tools_download_db_error"),
		})
		return
	}

	filename := fmt.Sprintf("customers-with-diamonds-%s.csv", time.Now().UTC().Format("20060102-1504"))

	_, err = b.SendDocument(ctx, &bot.SendDocumentParams{
		ChatID: cb.Message.Message.Chat.ID,
		Document: &models.InputFileUpload{
			Filename: filename,
			Data:     bytes.NewReader(buf.Bytes()),
		},
	})
	if err != nil {
		slog.Error("error sending csv document", "error", err)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    cb.Message.Message.Chat.ID,
			ParseMode: models.ParseModeHTML,
			Text:      h.translation.GetText(langCode, "admin_tools_download_db_error"),
		})
		return
	}
}

func (h Handler) AdminPricingCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_pricing_menu_title"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "admin_pricing_discounts_button"), CallbackData: CallbackAdminPricingDiscounts}},
				{{Text: h.translation.GetText(langCode, "admin_pricing_subscription_manage_button"), CallbackData: CallbackAdminSubscriptionManage}},
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			},
		},
	})
	if err != nil {
		slog.Error("error showing pricing menu", "error", err)
	}
}

func (h Handler) AdminPricingDiscountsCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_discounts_scope_title"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "admin_discounts_scope_global"), CallbackData: CallbackAdminDiscountScopePrefix + "?scope=global"}},
				{{Text: h.translation.GetText(langCode, "admin_discounts_scope_lite"), CallbackData: CallbackAdminDiscountScopePrefix + "?scope=lite"}},
				{{Text: h.translation.GetText(langCode, "admin_discounts_scope_premium"), CallbackData: CallbackAdminDiscountScopePrefix + "?scope=premium"}},
				{{Text: h.translation.GetText(langCode, "admin_discounts_scope_flush"), CallbackData: CallbackAdminDiscountFlush}},
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminPricing}},
			},
		},
	})
	if err != nil {
		slog.Error("error showing discounts scope menu", "error", err)
	}
}

func (h Handler) AdminSubscriptionManageCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	h.adminState.SetState(admin.StateAwaitingSubscriptionTargetID)

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_subscription_enter_telegram_id"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminPricing}},
			},
		},
	})
	if err != nil {
		slog.Error("error showing subscription management prompt", "error", err)
	}
}

func (h Handler) AdminSubscriptionActionCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	params := parseCallbackData(cb.Data)
	action := params["action"]
	idStr := params["id"]
	if action == "" || idStr == "" {
		return
	}

	targetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return
	}

	targetCustomer, err := h.customerRepository.FindByTelegramId(ctx, targetID)
	if err != nil || targetCustomer == nil {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    callbackMessage.Chat.ID,
			MessageID: callbackMessage.ID,
			Text:      h.translation.GetText(langCode, "admin_subscription_not_found"),
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminPricing}},
				},
			},
		})
		return
	}

	if action == "remove" {
		now := time.Now().UTC()
		updates := map[string]interface{}{
			"plan":      "free",
			"expire_at": now,
		}
		if err := h.customerRepository.UpdateFields(ctx, targetCustomer.ID, updates); err != nil {
			slog.Error("error removing subscription", "error", err)
			_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID:    callbackMessage.Chat.ID,
				MessageID: callbackMessage.ID,
				Text:      h.translation.GetText(langCode, "admin_subscription_update_error"),
				ParseMode: models.ParseModeHTML,
			})
			return
		}

		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    callbackMessage.Chat.ID,
			MessageID: callbackMessage.ID,
			Text:      h.translation.GetText(langCode, "admin_subscription_removed"),
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminPricing}},
				},
			},
		})
		return
	}

	// Assign or change plan, ask for duration.
	var plan string
	switch action {
	case "lite":
		plan = "lite"
	case "premium":
		plan = "premium"
	default:
		return
	}

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_subscription_choose_duration"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{
						Text:         h.translation.GetText(langCode, "month_1"),
						CallbackData: fmt.Sprintf("%s?id=%d&plan=%s&months=%d", CallbackAdminSubscriptionDurationPrefix, targetCustomer.TelegramID, plan, 1),
					},
					{
						Text:         h.translation.GetText(langCode, "month_12"),
						CallbackData: fmt.Sprintf("%s?id=%d&plan=%s&months=%d", CallbackAdminSubscriptionDurationPrefix, targetCustomer.TelegramID, plan, 12),
					},
				},
				{
					{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminPricing},
				},
			},
		},
	})
	if err != nil {
		slog.Error("error showing subscription duration menu", "error", err)
	}
}

func (h Handler) AdminSubscriptionDurationCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	params := parseCallbackData(cb.Data)
	idStr := params["id"]
	plan := params["plan"]
	monthsStr := params["months"]
	if idStr == "" || plan == "" || monthsStr == "" {
		return
	}

	targetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return
	}
	months, err := strconv.Atoi(monthsStr)
	if err != nil || (months != 1 && months != 12) {
		return
	}

	targetCustomer, err := h.customerRepository.FindByTelegramId(ctx, targetID)
	if err != nil || targetCustomer == nil {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    callbackMessage.Chat.ID,
			MessageID: callbackMessage.ID,
			Text:      h.translation.GetText(langCode, "admin_subscription_not_found"),
			ParseMode: models.ParseModeHTML,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminPricing}},
				},
			},
		})
		return
	}

	now := time.Now().UTC()
	days := config.DaysInMonth() * months
	expireAt := now.Add(time.Duration(days) * 24 * time.Hour)

	updates := map[string]interface{}{
		"plan":      plan,
		"expire_at": expireAt,
	}

	if err := h.customerRepository.UpdateFields(ctx, targetCustomer.ID, updates); err != nil {
		slog.Error("error updating subscription", "error", err)
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    callbackMessage.Chat.ID,
			MessageID: callbackMessage.ID,
			Text:      h.translation.GetText(langCode, "admin_subscription_update_error"),
			ParseMode: models.ParseModeHTML,
		})
		return
	}

	summary := fmt.Sprintf(
		h.translation.GetText(langCode, "admin_subscription_updated"),
		targetCustomer.TelegramID,
		plan,
		expireAt.Format("02.01.2006 15:04"),
	)

	_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      summary,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminPricing}},
			},
		},
	})
}

func (h Handler) AdminDiscountFlushCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	h.pricingService.FlushDiscounts()

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_discounts_flushed"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminPricingDiscounts}},
			},
		},
	})
	if err != nil {
		slog.Error("error sending discounts flushed message", "error", err)
	}
}

func (h Handler) AdminDiscountScopeCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	params := parseCallbackData(cb.Data)
	scope := params["scope"]
	if scope == "" {
		return
	}

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_discounts_timing_title"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: h.translation.GetText(langCode, "admin_discounts_timing_1h"), CallbackData: fmt.Sprintf("%s?scope=%s&duration=1h", CallbackAdminDiscountDurationPrefix, scope)},
				},
				{
					{Text: h.translation.GetText(langCode, "admin_discounts_timing_1d"), CallbackData: fmt.Sprintf("%s?scope=%s&duration=1d", CallbackAdminDiscountDurationPrefix, scope)},
				},
				{
					{Text: h.translation.GetText(langCode, "admin_discounts_timing_1w"), CallbackData: fmt.Sprintf("%s?scope=%s&duration=1w", CallbackAdminDiscountDurationPrefix, scope)},
				},
				{
					{Text: h.translation.GetText(langCode, "admin_discounts_timing_1m"), CallbackData: fmt.Sprintf("%s?scope=%s&duration=1m", CallbackAdminDiscountDurationPrefix, scope)},
				},
				{
					{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminPricingDiscounts},
				},
			},
		},
	})
	if err != nil {
		slog.Error("error showing discounts timing menu", "error", err)
	}
}

func (h Handler) AdminDiscountDurationCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	params := parseCallbackData(cb.Data)
	scope := params["scope"]
	duration := params["duration"]
	if scope == "" || duration == "" {
		return
	}

	// Build percentage buttons 10%..90%.
	var rows [][]models.InlineKeyboardButton
	row := []models.InlineKeyboardButton{}
	for pct := 10; pct <= 90; pct += 10 {
		btn := models.InlineKeyboardButton{
			Text:         fmt.Sprintf("%d%%", pct),
			CallbackData: fmt.Sprintf("%s?scope=%s&duration=%s&percent=%d", CallbackAdminDiscountPercentPrefix, scope, duration, pct),
		}
		row = append(row, btn)
		if len(row) == 3 {
			rows = append(rows, row)
			row = []models.InlineKeyboardButton{}
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: fmt.Sprintf("%s?scope=%s", CallbackAdminDiscountScopePrefix, scope)},
	})

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_discounts_percent_title"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: rows,
		},
	})
	if err != nil {
		slog.Error("error showing discounts percent menu", "error", err)
	}
}

func parseAdminDiscountDuration(value string) time.Duration {
	switch value {
	case "1h":
		return time.Hour
	case "1d":
		return 24 * time.Hour
	case "1w":
		return 7 * 24 * time.Hour
	case "1m":
		// Approximate one month as 30 days for discount timing purposes.
		return 30 * 24 * time.Hour
	default:
		return 0
	}
}

func scopeDisplayName(langCode string, scope pricing.Scope, translation *translation.Manager) string {
	switch scope {
	case pricing.ScopeGlobal:
		return translation.GetText(langCode, "admin_discounts_scope_name_global")
	case pricing.ScopeLite:
		return translation.GetText(langCode, "admin_discounts_scope_name_lite")
	case pricing.ScopePremium:
		return translation.GetText(langCode, "admin_discounts_scope_name_premium")
	default:
		return string(scope)
	}
}

func (h Handler) AdminDiscountPercentCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	params := parseCallbackData(cb.Data)
	scopeStr := params["scope"]
	durationStr := params["duration"]
	percentStr := params["percent"]
	if scopeStr == "" || durationStr == "" || percentStr == "" {
		return
	}

	var scope pricing.Scope
	switch scopeStr {
	case "global":
		scope = pricing.ScopeGlobal
	case "lite":
		scope = pricing.ScopeLite
	case "premium":
		scope = pricing.ScopePremium
	default:
		return
	}

	dur := parseAdminDiscountDuration(durationStr)
	if dur <= 0 {
		return
	}

	percent, err := strconv.Atoi(percentStr)
	if err != nil {
		return
	}

	h.pricingService.SetDiscount(scope, percent, dur)

	// Find the applied discount to get its precise expiration time.
	var expiresAt time.Time
	for _, d := range h.pricingService.ActiveDiscounts() {
		if d.Scope == scope && d.Percent == percent {
			expiresAt = d.ExpiresAt
			break
		}
	}
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(dur)
	}

	// Build a small preview of resulting prices (baseline from env, then discounted).
	previewLines := []string{}
	type combo struct {
		tier   string
		months int
		method pricing.Method
		label  string
	}
	combinations := []combo{
		{"lite", 1, pricing.MethodDirect, "Lite 1m Direct"},
		{"lite", 12, pricing.MethodDirect, "Lite 12m Direct"},
		{"premium", 1, pricing.MethodDirect, "Premium 1m Direct"},
		{"premium", 12, pricing.MethodDirect, "Premium 12m Direct"},
		{"lite", 1, pricing.MethodStars, "Lite 1m Stars"},
		{"lite", 12, pricing.MethodStars, "Lite 12m Stars"},
		{"premium", 1, pricing.MethodStars, "Premium 1m Stars"},
		{"premium", 12, pricing.MethodStars, "Premium 12m Stars"},
	}

	for _, c := range combinations {
		// Only show tiers affected by this scope (plus global).
		if scope == pricing.ScopeLite && c.tier != "lite" {
			continue
		}
		if scope == pricing.ScopePremium && c.tier != "premium" {
			continue
		}

		baseline := pricing.BaselinePriceForPreview(c.tier, c.months, c.method)

		if baseline <= 0 {
			continue
		}
		discounted := (baseline * (100 - percent)) / 100
		if discounted == baseline {
			continue
		}
		previewLines = append(previewLines, fmt.Sprintf("%s: %d → %d", c.label, baseline, discounted))
	}

	scopeName := scopeDisplayName(langCode, scope, h.translation)
	expiresAtStr := expiresAt.UTC().Format("2006-01-02 15:04 MST")

	summary := fmt.Sprintf(h.translation.GetText(langCode, "admin_discounts_applied"), percent, scopeName, durationStr, expiresAtStr)
	if len(previewLines) > 0 {
		summary = summary + "\n\n" + strings.Join(previewLines, "\n")
	}

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      summary,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminPricingDiscounts}},
			},
		},
	})
	if err != nil {
		slog.Error("error sending discounts summary", "error", err)
	}
}

