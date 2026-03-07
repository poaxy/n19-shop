package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/config"
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

