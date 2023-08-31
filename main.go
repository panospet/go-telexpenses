package main

import (
	"context"
	"log"
	"log/slog"
	"strconv"
	"strings"

	"github.com/caarlos0/env/v9"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v4/pgxpool"

	"go-telexpenses/internal/expense"
)

const (
	ExpensePsilika       = "Ψιλικά"
	ExpenseFood          = "Φαγητό"
	ExpenseLesson        = "Ψ/Γ/Πιλάτες"
	ExpensePurchases     = "Αγορές"
	ExpenseCoffee        = "Καφέδες"
	ExpenseEntertainment = "Διασκέδαση"
	ExpenseGas           = "Βενζίνες"
	ExpenseBills         = "Λογαριασμοί"
	ExpenseSupermarket   = "Σούπερ-Λαική"
	ExpenseOther         = "Άλλο"
)

var categoryKeyboard = tgbotapi.NewReplyKeyboard(
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(ExpensePsilika),
		tgbotapi.NewKeyboardButton(ExpenseFood),
		tgbotapi.NewKeyboardButton(ExpenseLesson),
		tgbotapi.NewKeyboardButton(ExpensePurchases),
		tgbotapi.NewKeyboardButton(ExpenseCoffee),
	),
	tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(ExpenseEntertainment),
		tgbotapi.NewKeyboardButton(ExpenseGas),
		tgbotapi.NewKeyboardButton(ExpenseBills),
		tgbotapi.NewKeyboardButton(ExpenseSupermarket),
		tgbotapi.NewKeyboardButton(ExpenseOther),
	),
)

type config struct {
	TelegramToken string `env:"TELEGRAM_APITOKEN,required"`
	PostgresDsn   string `env:"POSTGRES_DSN,required"`
}

type Ongoing struct {
	UserId   int64
	State    string
	Category string
	Amount   float64
	Comment  string
}

func main() {
	ctx := context.Background()

	cfg := config{}
	if err := env.Parse(&cfg); err != nil {
		log.Fatal(err)
	}

	conn, err := pgxpool.Connect(ctx, cfg.PostgresDsn)
	if err != nil {
		log.Fatalf("error connecting to database: %v", err)
	}
	defer conn.Close()
	if err := conn.Ping(ctx); err != nil {
		log.Fatalf("error pinging database: %v", err)
	}

	repo := expense.NewRepo(ctx, conn)

	bot, err := tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		log.Fatalf("Error creating bot: %v", err)
	}

	bot.Debug = true

	slog.Info("telegram bot authorized successfully", slog.String("account", bot.Self.UserName))

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	var ongoing *Ongoing

	for update := range updates {
		if update.Message == nil { // ignore non-Message updates
			continue
		}

		if update.Message.IsCommand() {
			switch update.Message.Command() {

			case "expense":
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Επέλεξε κατηγορία")
				msg.ReplyMarkup = categoryKeyboard
				if _, err := bot.Send(msg); err != nil {
					slog.Warn("cannot send message", slog.String("error", err.Error()))
				}
				ongoing = &Ongoing{
					UserId: update.Message.From.ID,
					State:  "user_chose_category",
				}

			default:
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Δεν καταλαβαίνω τι λες")
				if _, err := bot.Send(msg); err != nil {
					slog.Warn("cannot send message", slog.String("error", err.Error()))
				}
			}

			continue
		}

		if update.CallbackQuery != nil {
			callback := tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackQuery.Data)
			if _, err := bot.Request(callback); err != nil {
				panic(err)
			}

			msg := tgbotapi.NewMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Data+" επιλέχθηκε")
			if _, err := bot.Send(msg); err != nil {
				panic(err)
			}

			continue
		}

		if ongoing != nil && update.Message.From.ID != ongoing.UserId {
			msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Εσένα δεν σου μιλάω (ακόμα)")
			if _, err := bot.Send(msg); err != nil {
				slog.Warn("cannot send message", slog.String("error", err.Error()))
			}
			continue
		}

		if ongoing != nil {
			switch ongoing.State {
			case "user_chose_category":
				ongoing.Category = update.Message.Text
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "πόσα ξόδεψες;")
				if _, err := bot.Send(msg); err != nil {
					slog.Warn("cannot send message", slog.String("error", err.Error()))
				}
				ongoing.State = "user_was_asked_amount"

			case "user_was_asked_amount":
				floatStr := strings.Replace(update.Message.Text, ",", ".", -1)
				ongoing.Amount, err = strconv.ParseFloat(floatStr, 64)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Δεν μπορώ να καταλάβω πόσα ξόδεψες. Πες μου ξανά.")
					if _, err := bot.Send(msg); err != nil {
						slog.Warn("cannot send message", slog.String("error", err.Error()))
					}
					continue
				}
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Έχεις κάποιο σχόλιο;")
				if _, err := bot.Send(msg); err != nil {
					slog.Warn("cannot send message", slog.String("error", err.Error()))
				}
				ongoing.State = "user_was_asked_comment"

			case "user_was_asked_comment":
				ongoing.Comment = update.Message.Text
				err := repo.AddExpense(ctx, expense.Expense{
					UserId:   ongoing.UserId,
					Category: ongoing.Category,
					Amount:   ongoing.Amount,
					Comment:  ongoing.Comment,
				})
				if err != nil {
					slog.Warn("cannot add expense", slog.String("error", err.Error()))
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Κάτι πήγε λάθος. Προσπάθησε ξανά.")
					if _, err := bot.Send(msg); err != nil {
						slog.Warn("cannot send message", slog.String("error", err.Error()))
					}
					ongoing = nil
					continue
				}
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Ευχαριστώ! Τα κατέγραψα.")
				if _, err := bot.Send(msg); err != nil {
					slog.Warn("cannot send message", slog.String("error", err.Error()))
				}
				ongoing = nil
			}
		}
	}
}
