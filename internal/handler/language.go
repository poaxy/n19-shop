package handler

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"log/slog"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

type LanguageOption struct {
	Code    string
	NameKey string
}

var supportedLanguages = []LanguageOption{
	{Code: "en", NameKey: "language_name_en"},
	{Code: "ru", NameKey: "language_name_ru"},
	{Code: "pt-BR", NameKey: "language_name_pt_br"},
	{Code: "fa", NameKey: "language_name_fa"},
}

const defaultLanguageCode = "en"

func normalizeLanguageCode(code string) string {
	if code == "" {
		return defaultLanguageCode
	}

	code = strings.TrimSpace(code)
	lower := strings.ToLower(code)

	switch lower {
	case "en", "en-us", "en-gb":
		return "en"
	case "ru", "ru-ru":
		return "ru"
	case "pt", "pt-br", "pt_br", "pt-pt":
		return "pt-BR"
	case "fa", "fa-ir", "fa_ir", "fa-fa":
		return "fa"
	default:
		return defaultLanguageCode
	}
}

func (h *Handler) getUserLanguage(customer *database.Customer, from *models.User) string {
	if customer != nil && customer.Language != "" {
		return normalizeLanguageCode(customer.Language)
	}

	if from != nil && from.LanguageCode != "" {
		return normalizeLanguageCode(from.LanguageCode)
	}

	return defaultLanguageCode
}
func isSupportedLanguage(code string) bool {
	for _, opt := range supportedLanguages {
		if opt.Code == code {
			return true
		}
	}
	return false
}

func (h Handler) LanguageMenuCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb == nil || cb.Message.Message == nil {
		return
	}

	customer, err := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	if err != nil {
		slog.Error("error finding customer for language menu", "error", err)
		return
	}

	langCode := h.getUserLanguage(customer, &cb.From)
	chatID := cb.Message.Message.Chat.ID

	var keyboard [][]models.InlineKeyboardButton
	for _, opt := range supportedLanguages {
		text := h.translation.GetText(langCode, opt.NameKey)
		if opt.Code == langCode {
			text = "✅ " + text
		}
		keyboard = append(keyboard, []models.InlineKeyboardButton{
			{Text: text, CallbackData: CallbackLanguageSelectPrefix + opt.Code},
		})
	}

	keyboard = append(keyboard, []models.InlineKeyboardButton{
		{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart},
	})

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: cb.Message.Message.ID,
		Text:      h.translation.GetText(langCode, "language_menu_title"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	})
	if err != nil {
		slog.Error("error sending language menu", "error", err)
	}
}

func (h Handler) LanguageSelectCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb == nil || cb.Message.Message == nil {
		return
	}

	data := cb.Data
	if !strings.HasPrefix(data, CallbackLanguageSelectPrefix) {
		return
	}

	selectedCode := normalizeLanguageCode(strings.TrimPrefix(data, CallbackLanguageSelectPrefix))
	if !isSupportedLanguage(selectedCode) {
		return
	}

	customer, err := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	if err != nil {
		slog.Error("error finding customer for language select", "error", err)
		return
	}

	if customer == nil {
		newCustomer, err := h.customerRepository.Create(ctx, &database.Customer{
			TelegramID: cb.From.ID,
			Language:   selectedCode,
		})
		if err != nil {
			slog.Error("error creating customer for language select", "error", err)
			return
		}
		customer = newCustomer
	} else {
		updates := map[string]interface{}{
			"language": selectedCode,
		}
		if err := h.customerRepository.UpdateFields(ctx, customer.ID, updates); err != nil {
			slog.Error("error updating customer language", "error", err)
			return
		}
		customer.Language = selectedCode
	}

	langCode := h.getUserLanguage(customer, &cb.From)
	chatID := cb.Message.Message.Chat.ID

	var text string
	var keyboard [][]models.InlineKeyboardButton

	if cb.From.ID == config.GetAdminTelegramId() {
		keyboard = h.buildAdminStartKeyboard(langCode)
		text = h.translation.GetText(langCode, "admin_menu_title")
	} else {
		keyboard = h.buildStartKeyboard(customer, langCode)
		success := h.translation.GetText(langCode, "language_set_success")
		greeting := h.translation.GetText(langCode, "greeting")
		text = success + "\n\n" + greeting
	}

	_, err = b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: cb.Message.Message.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	})
	if err != nil {
		slog.Error("error sending language updated menu", "error", err)
	}
}

