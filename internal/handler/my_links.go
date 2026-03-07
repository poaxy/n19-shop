package handler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h Handler) MyLinksCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	chatID := cb.Message.Message.Chat.ID

	customer, err := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	if err != nil {
		slog.Error("error finding customer for my links", "error", err)
		return
	}

	langCode := h.getUserLanguage(customer, &cb.From)

	var text string
	if customer == nil || customer.SubscriptionLink == nil || customer.ExpireAt == nil || !customer.ExpireAt.After(time.Now()) {
		text = h.translation.GetText(langCode, "my_links_empty")
	} else {
		expireAt := customer.ExpireAt.Format("02.01.2006 15:04")
		link := ""
		if customer.SubscriptionLink != nil {
			link = *customer.SubscriptionLink
		}
		diamonds := customer.Diamonds
		text = fmt.Sprintf(
			h.translation.GetText(langCode, "my_links_info"),
			link,
			expireAt,
			diamonds,
		)
	}

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: cb.Message.Message.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackProfile},
				},
			},
		},
	})
	if err != nil {
		slog.Error("error sending my links info", "error", err)
	}
}

