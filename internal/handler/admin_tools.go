package handler

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
)

func (h Handler) AdminToolsCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	langCode := cb.From.LanguageCode
	callbackMessage := cb.Message.Message

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_tools_placeholder"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			},
		},
	})
	if err != nil {
		slog.Error("error showing admin tools placeholder", "error", err)
	}
}

