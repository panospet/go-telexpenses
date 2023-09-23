package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/agnivade/levenshtein"
	"github.com/caarlos0/env/v9"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/gosimple/slug"
	"github.com/jackc/pgx/v4/pgxpool"
	_ "github.com/lib/pq"

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

var categories = []string{
	ExpensePsilika,
	ExpenseFood,
	ExpenseLesson,
	ExpensePurchases,
	ExpenseCoffee,
	ExpenseEntertainment,
	ExpenseGas,
	ExpenseBills,
	ExpenseSupermarket,
	ExpenseOther,
}

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
	MigrationsDir string `env:"MIGRATIONS_DIR" envDefault:"./migrations"`
}

type Ongoing struct {
	UserId   int64
	State    string
	Category string
	Amount   float64
	Comment  string
}

func thisYear() int {
	return time.Now().Year()
}

func thisMonth() int {
	return int(time.Now().Month())
}

var bot *tgbotapi.BotAPI

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config{}
	if err := env.Parse(&cfg); err != nil {
		log.Fatal(err)
	}

	// wait for postgres to be ready
	time.Sleep(2 * time.Second)

	conn, err := pgxpool.Connect(ctx, cfg.PostgresDsn)
	if err != nil {
		log.Fatalf("error connecting to database: %v", err)
	}
	defer conn.Close()
	if err := conn.Ping(ctx); err != nil {
		log.Fatalf("error pinging database: %v", err)
	}
	err = migrateDb(cfg)
	if err != nil {
		log.Fatalf("error migrating database: %v", err)
	}

	repo := expense.NewRepo(ctx, conn)

	bot, err = tgbotapi.NewBotAPI(cfg.TelegramToken)
	if err != nil {
		log.Fatalf("Error creating bot: %v", err)
	}

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

			case "cancel":
				ongoing = nil
				sendSimpleMessage(update.Message.Chat.ID, "Έγινε ακύρωση. Όλα καλά.")
				continue

			case "new":
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Καινούριο έξοδο ε; \n Επίλεξε κατηγορία:")
				msg.ReplyMarkup = categoryKeyboard
				if _, err := bot.Send(msg); err != nil {
					slog.Warn("cannot send message", slog.String("error", err.Error()))
				}
				ongoing = &Ongoing{
					UserId: update.Message.From.ID,
					State:  "user_chose_category",
				}

			case "month":
				expenses, err := repo.GetExpenses(
					ctx,
					expense.Filter{
						Year:  thisYear(),
						Month: thisMonth(),
					},
				)
				if err != nil {
					slog.Error("cannot get expenses", slog.String("error", err.Error()))
					sendSimpleMessage(update.Message.Chat.ID, "κάτι πήγε στραβά.")
					continue
				}

				prettyPrintExpenses(update.Message.Chat.ID, expenses)
				continue

			case "month_specific":
				sendSimpleMessage(update.Message.Chat.ID, "γράψε τον χρόνο, τον μήνα και την κατηγορία (π.χ. 2021 5 Ψιλικά)")
				ongoing = &Ongoing{
					UserId: update.Message.From.ID,
					State:  "user_was_asked_for_specific_month",
				}
				continue

			default:
				msg := tgbotapi.NewMessage(
					update.Message.Chat.ID,
					"Δεν καταλαβαίνω τι λες. "+
						"Μπορείς όμως να κάνεις τα εξής: \n"+
						"- `/new` για να δηλώσεις ένα καινούριο έξοδο \n"+
						"- `/month` για να δεις τι έχεις ξοδέψει σύνολο αυτόν τον μήνα \n"+
						"- `/month_specific` για να δεις τι έχεις ξοδέψει βάσει χρόνου, μήνα και κατηγορίας \n",
				)
				msg.ParseMode = tgbotapi.ModeMarkdown
				if _, err := bot.Send(msg); err != nil {
					slog.Error("cannot send message", slog.String("error", err.Error()))
				}
			}

			continue
		}

		if ongoing != nil && update.Message.From.ID != ongoing.UserId {
			sendSimpleMessage(update.Message.Chat.ID, "Εσένα δεν σου μιλάω (ακόμα)")
			continue
		}

		if ongoing != nil {
			switch ongoing.State {
			case "user_chose_category":
				ongoing.Category = update.Message.Text
				sendSimpleMessage(update.Message.Chat.ID, "πόσα ξόδεψες;")
				ongoing.State = "user_was_asked_amount"

			case "user_was_asked_amount":
				floatStr := strings.Replace(update.Message.Text, ",", ".", -1)
				log.Printf("floatStr: %s", floatStr)
				ongoing.Amount, err = strconv.ParseFloat(floatStr, 32)
				if err != nil {
					sendSimpleMessage(update.Message.Chat.ID, "Δεν μπορώ να καταλάβω πόσα ξόδεψες. Πες μου ξανά.")
					continue
				}
				log.Printf("float amount: %f", ongoing.Amount)
				sendSimpleMessage(update.Message.Chat.ID, "Δώσε μου και ένα σχόλιο")
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
					slog.Error("cannot add expense", slog.String("error", err.Error()))
					sendSimpleMessage(update.Message.Chat.ID, "Κάτι πήγε λάθος. Προσπάθησε ξανά.")
					continue
				}
				sendSimpleMessage(update.Message.Chat.ID, "Ευχαριστώ! Τα κατέγραψα.")
				ongoing = nil

			case "user_was_asked_for_specific_month":
				parts := strings.Split(update.Message.Text, " ")
				var year, month int
				var category string
				var err error
				year, err = strconv.Atoi(parts[0])
				if err != nil {
					sendSimpleMessage(update.Message.Chat.ID, "Δεν μπορώ να καταλάβω τον χρόνο. Πες μου ξανά.")
					continue
				}
				if len(parts) > 1 {
					month, err = strconv.Atoi(parts[1])
					if err != nil {
						sendSimpleMessage(update.Message.Chat.ID, "Δεν μπορώ να καταλάβω τον μήνα. Πες μου ξανά.")
						continue
					}
				}
				if len(parts) > 2 {
					category = fuzzyMatchCategory(parts[2])
					if category == "" {
						sendSimpleMessage(update.Message.Chat.ID, "Η συγκεκριμένη κατηγορία δεν βρέθηκε, υπολογίζω για όλες.")
					}
				}
				expenses, err := repo.GetExpenses(ctx, expense.Filter{
					Year:     year,
					Month:    month,
					Category: category,
				})
				if err != nil {
					slog.Error("cannot get expenses", slog.String("error", err.Error()))
					sendSimpleMessage(update.Message.Chat.ID, "Κάτι πήγε λάθος. Προσπάθησε ξανά.")
					continue
				}

				prettyPrintExpenses(update.Message.Chat.ID, expenses)
				continue
			}
		}
	}
}

func fuzzyMatchCategory(category string) string {
	slugged := slug.Make(strings.ToLower(category))
	for _, c := range categories {
		if levenshtein.ComputeDistance(slugged, slug.Make(strings.ToLower(c))) < 3 {
			return c
		}
	}

	return ""
}

func prettyPrintExpenses(chatId int64, expenses []expense.Expense) {
	if len(expenses) == 0 {
		sendSimpleMessage(chatId, "Δεν βρήκα τίποτα.")
		return
	}
	expensesMap := expense.SumExpensesByCategory(expenses)
	var msgText strings.Builder
	msgText.WriteString("Έξοδα για τον μήνα αυτό: \n")
	for category, amount := range expensesMap {
		msgText.WriteString(fmt.Sprintf("- %s: %.2f€ \n", category, amount))
	}
	sendSimpleMessage(chatId, msgText.String())
}

func sendSimpleMessage(chatId int64, message string) {
	msg := tgbotapi.NewMessage(chatId, message)
	if _, err := bot.Send(msg); err != nil {
		slog.Error("cannot send message", slog.String("error", err.Error()))
	}
}

func migrateDb(config config) error {
	pg, err := sql.Open("postgres", config.PostgresDsn)
	if err != nil {
		return fmt.Errorf("sql.Open error: %s", err)
	}
	defer pg.Close()
	driver, err := postgres.WithInstance(pg, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("cannot init go-migrate: %s", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+config.MigrationsDir, "postgres", driver)
	if err != nil {
		return fmt.Errorf("cannot create migrate.NewWithDatabaseInstance: %s", err)
	}
	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("cannot migrate up: %s", err)
	}

	log.Println("all database migrations are complete")

	return nil
}
