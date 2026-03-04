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
	if customer != nil && customer.SubscriptionLink != nil && customer.ExpireAt != nil && customer.ExpireAt.After(time.Now()) {
		plan = h.translation.GetText(langCode, "profile_plan_free")
	}

	text := fmt.Sprintf(
		h.translation.GetText(langCode, "profile_info"),
		cb.From.ID,
		username,
		role,
		plan,
		expireAtStr,
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

