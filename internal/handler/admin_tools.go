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

