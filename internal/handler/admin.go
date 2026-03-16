package handler

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"remnawave-tg-shop-bot/internal/admin"
	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/utils"
)

func (h Handler) AdminNotifyCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
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
		Text:      h.translation.GetText(langCode, "admin_notify_menu_title"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "admin_notify_all_button"), CallbackData: CallbackAdminNotifyAll}},
				{{Text: h.translation.GetText(langCode, "admin_notify_direct_button"), CallbackData: CallbackAdminNotifyDirect}},
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			},
		},
	})
	if err != nil {
		slog.Error("error showing admin notify menu", "error", err)
	}
}

func (h Handler) AdminNotifyAllCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	h.adminState.SetState(admin.StateAwaitingBroadcastText)
	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_notify_all_prompt"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			},
		},
	})
	if err != nil {
		slog.Error("error showing admin notify all prompt", "error", err)
	}
}

func (h Handler) AdminNotifyDirectCallbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb.From.ID != config.GetAdminTelegramId() {
		return
	}

	h.adminState.SetState(admin.StateAwaitingDirectTargetID)
	customer, _ := h.customerRepository.FindByTelegramId(ctx, cb.From.ID)
	langCode := h.getUserLanguage(customer, &cb.From)
	callbackMessage := cb.Message.Message

	_, err := b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:    callbackMessage.Chat.ID,
		MessageID: callbackMessage.ID,
		Text:      h.translation.GetText(langCode, "admin_notify_direct_id_prompt"),
		ParseMode: models.ParseModeHTML,
		ReplyMarkup: models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackStart}},
			},
		},
	})
	if err != nil {
		slog.Error("error showing admin notify direct id prompt", "error", err)
	}
}

// AdminMessageHandler should be registered for generic text messages before user handlers.
// It routes messages through the admin state machine when the sender is the admin.
func (h Handler) AdminMessageHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	msg := update.Message
	if msg.From.ID != config.GetAdminTelegramId() {
		return
	}

	currentState := h.adminState.GetState()
	if currentState == admin.StateIdle {
		return
	}

	customer, _ := h.customerRepository.FindByTelegramId(ctx, msg.From.ID)
	langCode := h.getUserLanguage(customer, msg.From)

	switch currentState {
	case admin.StateAwaitingBroadcastText:
		text := msg.Text
		slog.Info("admin broadcast started")

		ids, err := h.customerRepository.ListAllTelegramIds(ctx)
		if err != nil {
			slog.Error("error listing telegram ids for broadcast", "error", err)
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    msg.Chat.ID,
				ParseMode: models.ParseModeHTML,
				Text:      h.translation.GetText(langCode, "admin_notify_error"),
			})
			h.adminState.SetState(admin.StateIdle)
			return
		}

		sent := 0
		for _, id := range ids {
			_, sendErr := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    id,
				ParseMode: models.ParseModeHTML,
				Text:      text,
			})
			if sendErr != nil {
				slog.Error("error sending broadcast message", "error", sendErr, "telegramId", utils.MaskHalfInt64(id))
				continue
			}
			sent++
		}

		h.adminState.SetState(admin.StateIdle)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    msg.Chat.ID,
			ParseMode: models.ParseModeHTML,
			Text:      h.translation.GetText(langCode, "admin_notify_all_done"),
		})
		slog.Info("admin broadcast finished", "sent", sent)

	case admin.StateAwaitingDirectTargetID:
		targetID, err := utils.ParseInt64(msg.Text)
		if err != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    msg.Chat.ID,
				ParseMode: models.ParseModeHTML,
				Text:      h.translation.GetText(langCode, "admin_notify_direct_id_invalid"),
			})
			return
		}

		customer, err := h.customerRepository.FindByTelegramId(ctx, targetID)
		if err != nil || customer == nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    msg.Chat.ID,
				ParseMode: models.ParseModeHTML,
				Text:      h.translation.GetText(langCode, "admin_notify_direct_not_found"),
			})
			return
		}

		h.adminState.SetDirectTarget(targetID)
		h.adminState.SetState(admin.StateAwaitingDirectMessage)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    msg.Chat.ID,
			ParseMode: models.ParseModeHTML,
			Text:      h.translation.GetText(langCode, "admin_notify_direct_message_prompt"),
		})

	case admin.StateAwaitingDirectMessage:
		targetID := h.adminState.GetDirectTarget()
		if targetID == 0 {
			h.adminState.SetState(admin.StateIdle)
			return
		}

		text := msg.Text
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    targetID,
			ParseMode: models.ParseModeHTML,
			Text:      text,
		})
		if err != nil {
			slog.Error("error sending direct admin message", "error", err, "telegramId", utils.MaskHalfInt64(targetID))
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    msg.Chat.ID,
				ParseMode: models.ParseModeHTML,
				Text:      h.translation.GetText(langCode, "admin_notify_error"),
			})
			h.adminState.SetState(admin.StateIdle)
			return
		}

		h.adminState.SetState(admin.StateIdle)
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    msg.Chat.ID,
			ParseMode: models.ParseModeHTML,
			Text:      h.translation.GetText(langCode, "admin_notify_direct_done"),
		})
		slog.Info("admin direct message sent", "targetId", utils.MaskHalfInt64(targetID))

	case admin.StateAwaitingSubscriptionTargetID:
		targetID, err := utils.ParseInt64(msg.Text)
		if err != nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    msg.Chat.ID,
				ParseMode: models.ParseModeHTML,
				Text:      h.translation.GetText(langCode, "admin_subscription_invalid_id"),
			})
			return
		}

		targetCustomer, err := h.customerRepository.FindByTelegramId(ctx, targetID)
		if err != nil || targetCustomer == nil {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    msg.Chat.ID,
				ParseMode: models.ParseModeHTML,
				Text:      h.translation.GetText(langCode, "admin_subscription_not_found"),
			})
			return
		}

		h.adminState.SetSubscriptionTargetTelegram(targetID)

		now := time.Now().UTC()
		activePlan := getActivePlan(targetCustomer, now)

		planLabel := h.translation.GetText(langCode, "profile_plan_none")
		switch activePlan {
		case "free":
			planLabel = h.translation.GetText(langCode, "profile_plan_free")
		case "lite":
			planLabel = h.translation.GetText(langCode, "plan_lite_button")
		case "premium":
			planLabel = h.translation.GetText(langCode, "plan_premium_button")
		}

		expireAtStr := h.translation.GetText(langCode, "profile_no_subscription")
		if targetCustomer.ExpireAt != nil {
			expireAtStr = targetCustomer.ExpireAt.Format("02.01.2006 15:04")
		}

		summary := fmt.Sprintf(
			h.translation.GetText(langCode, "admin_subscription_summary"),
			targetCustomer.TelegramID,
			planLabel,
			expireAtStr,
		)

		var actionRow []models.InlineKeyboardButton
		if activePlan == "lite" || activePlan == "premium" || activePlan == "free" {
			actionRow = append(actionRow, models.InlineKeyboardButton{
				Text:         h.translation.GetText(langCode, "admin_subscription_remove_button"),
				CallbackData: fmt.Sprintf("%s?action=remove&id=%d", CallbackAdminSubscriptionActionPrefix, targetCustomer.TelegramID),
			})
		}

		actionRow = append(actionRow,
			models.InlineKeyboardButton{
				Text:         h.translation.GetText(langCode, "admin_subscription_assign_lite_button"),
				CallbackData: fmt.Sprintf("%s?action=lite&id=%d", CallbackAdminSubscriptionActionPrefix, targetCustomer.TelegramID),
			},
			models.InlineKeyboardButton{
				Text:         h.translation.GetText(langCode, "admin_subscription_assign_premium_button"),
				CallbackData: fmt.Sprintf("%s?action=premium&id=%d", CallbackAdminSubscriptionActionPrefix, targetCustomer.TelegramID),
			},
		)

		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    msg.Chat.ID,
			ParseMode: models.ParseModeHTML,
			Text:      summary,
			ReplyMarkup: models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					actionRow,
					{
						{Text: h.translation.GetText(langCode, "back_button"), CallbackData: CallbackAdminPricing},
					},
				},
			},
		})
		h.adminState.SetState(admin.StateIdle)
	}
}

