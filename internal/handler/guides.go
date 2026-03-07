package handler

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func (h Handler) GuidesCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    cb.Message.Message.Chat.ID,
		MessageID: cb.Message.Message.ID,
		Text:      h.translation.GetText(langCode, "guides_title"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: h.translation.GetText(langCode, "guide_how_to_connect_button"), CallbackData: CallbackGuideHowTo},
				},
				{
					{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
				},
			},
		},
	})
	if err != nil {
		slog.Error("error sending guides menu", "error", err)
	}
}

func (h Handler) GuideHowToConnectCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    cb.Message.Message.Chat.ID,
		MessageID: cb.Message.Message.ID,
		Text:      h.translation.GetText(langCode, "guide_how_to_connect_text"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{
					{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackGuides},
				},
			},
		},
	})
	if err != nil {
		slog.Error("error sending how-to-connect guide", "error", err)
	}
}

