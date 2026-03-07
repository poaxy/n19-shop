package handler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"remnawave-tg-shop-bot/internal/config"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h Handler) ProfileCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	langCode := cb.From.LanguageCode
	chatID := cb.Message.Message.Chat.ID

	customer, err := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	if err != nil {
		slog.Error("error finding customer for profile", "error", err)
		return
	}

	role := "customer"
	if cb.From.ID == config.GetAdminTelegramId() {
		role = "admin"
	}

	username := cb.From.Username
	if username == "" {
		username = "—"
	}

	var expireAtStr string
	if customer != nil && customer.ExpireAt != nil {
		expireAtStr = customer.ExpireAt.Format("02.01.2006 15:04")
	} else {
		expireAtStr = h.translation.GetText(langCode, "profile_no_subscription")
	}

	plan := h.translation.GetText(langCode, "profile_plan_none")
	if customer != nil {
		switch customer.Plan {
		case "free":
			plan = h.translation.GetText(langCode, "profile_plan_free")
		case "lite":
			plan = h.translation.GetText(langCode, "plan_lite_button")
		case "premium":
			plan = h.translation.GetText(langCode, "plan_premium_button")
		}
	}

	diamonds := 0
	if customer != nil {
		diamonds = customer.Diamonds
	}

	totalInvites := 0
	successfulInvites := 0
	if customer != nil {
		if count, err := h.referralRepository.CountByReferrer(ctx, customer.TelegramID); err == nil {
			totalInvites = count
		}
		if count, err := h.referralRepository.CountSuccessfulByReferrer(ctx, customer.TelegramID); err == nil {
			successfulInvites = count
		}
	}

	text := fmt.Sprintf(
		h.translation.GetText(langCode, "profile_info"),
		cb.From.ID,
		username,
		role,
		plan,
		expireAtStr,
		diamonds,
		totalInvites,
		successfulInvites,
	)

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: cb.Message.Message.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: h.translation.GetText(langCode, "my_links_button"), CallbackData: CallbackMyLinks},
				},
				{
					{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
				},
			},
		},
	})
	if err != nil {
		slog.Error("error sending profile info", "error", err)
	}
}

