package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/precheckoutquery"
)

var userBalances = make(map[int64]int)

func main() {

	// Get token from the environment variable
	token := os.Getenv("TOKEN")
	if token == "" {
		panic("TOKEN environment variable is empty")
	}

	// This MUST be an HTTPS URL for telegram to accept it.
	webappURL := os.Getenv("URL")
	if webappURL == "" {
		panic("URL environment variable is empty")
	}

	// Get the webhook secret from the environment variable.
	webhookSecret := os.Getenv("WEBHOOK_SECRET")
	if webhookSecret == "" {
		panic("WEBHOOK_SECRET environment variable is empty")
	}

	// Create our bot.
	b, err := gotgbot.NewBot(token, nil)
	if err != nil {
		panic("failed to create new bot: " + err.Error())
	}

	// Create updater and dispatcher to handle updates in a simple manner.
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		// If an error is returned by a handler, log it and continue going.
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Println("an error occurred while handling update:", err.Error())
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})
	updater := ext.NewUpdater(dispatcher, nil)

	// /start command to introduce the bot and send the URL
	dispatcher.AddHandler(handlers.NewCommand("start", func(b *gotgbot.Bot, ctx *ext.Context) error {
		// We can wrap commands with anonymous functions to pass in extra variables, like the webapp URL, or other
		// configuration.
		return start(b, ctx, webappURL)
	}))
	// /buy command to create an invoice
	dispatcher.AddHandler(handlers.NewCommand("buy", func(b *gotgbot.Bot, ctx *ext.Context) error {
		return buy(b, ctx)
	}))
	dispatcher.AddHandler(handlers.NewPreCheckoutQuery(precheckoutquery.All, preCheckout))
	dispatcher.AddHandler(handlers.NewMessage(message.SuccessfulPayment, paymentComplete))
	// We add the bot webhook to our updater, such that we can populate the updater's http.Handler.
	err = updater.AddWebhook(b, b.Token, &ext.AddWebhookOpts{SecretToken: webhookSecret})
	if err != nil {
		panic("Failed to add bot webhooks to updater: " + err.Error())
	}

	// We select a subpath to specify where the updater handler is found on the http.Server.
	updaterSubpath := "/bots/"
	err = updater.SetAllBotWebhooks(webappURL+updaterSubpath, &gotgbot.SetWebhookOpts{
		MaxConnections:     100,
		DropPendingUpdates: true,
		SecretToken:        webhookSecret,
	})
	if err != nil {
		panic("Failed to set bot webhooks: " + err.Error())
	}

	// Setup new HTTP server mux to handle different paths.
	mux := http.NewServeMux()
	// This serves the home page.
	mux.HandleFunc("/", index(webappURL))
	// This serves our "validation" API, which checks if the input data is valid.
	mux.HandleFunc("/validate", validate(token))
	// This serves the updater's webhook handler.
	mux.HandleFunc(updaterSubpath, updater.GetHandlerFunc(updaterSubpath))
	mux.HandleFunc("/get-balance", getBalanceHandler)
	mux.HandleFunc("/create-invoice", createInvoiceHandler)
	server := http.Server{
		Handler: mux,
		Addr:    "0.0.0.0:8080",
	}

	log.Printf("%s has been started...\n", b.User.Username)
	// Start the webserver displaying the page.
	// Note: ListenAndServe is a blocking operation, so we don't need to call updater.Idle() here.
	if err := server.ListenAndServe(); err != nil {
		panic("failed to listen and serve: " + err.Error())
	}
}

// start introduces the bot.
func start(b *gotgbot.Bot, ctx *ext.Context, webappURL string) error {
	_, err := ctx.EffectiveMessage.Reply(b, fmt.Sprintf("Hello, I'm @%s.\nYou can use me to run a (very) simple telegram webapp demo!", b.User.Username), &gotgbot.SendMessageOpts{
		ParseMode: "HTML",
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{
			InlineKeyboard: [][]gotgbot.InlineKeyboardButton{{
				{Text: "Press me", WebApp: &gotgbot.WebAppInfo{Url: webappURL}},
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to send start message: %w", err)
	}
	return nil
}
func buy(b *gotgbot.Bot, ctx *ext.Context) error {

	invoice := gotgbot.SendInvoiceOpts{
		ProviderToken: "",
	}

	// Gửi invoice
	_, err := b.SendInvoice(ctx.EffectiveChat.Id, "Purchase 1 Telegram Star", "Invoice for Telegram Star", "payload", "XTR", []gotgbot.LabeledPrice{
		{Label: "Telegram Star", Amount: 1},
	}, &invoice)
	if err != nil {
		return fmt.Errorf("failed to send invoice: %w", err)
	}
	return nil
}

func createInvoiceHandler(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var reqBody struct {
		Amount int   `json:"amount"`
		UserID int64 `json:"user_id"`
	}
	err := decoder.Decode(&reqBody)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	b, err := gotgbot.NewBot(os.Getenv("TOKEN"), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	invoice := gotgbot.CreateInvoiceLinkOpts{
		ProviderToken: "",
	}

	// Tạo invoice và lấy URL
	inv, err := b.CreateInvoiceLink("Purchase 1 Telegram Star", "Invoice for Telegram Star", "payload", "XTR", []gotgbot.LabeledPrice{
		{Label: "Telegram Star", Amount: 1},
	}, &invoice)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Trả về URL của invoice
	resp := map[string]string{"invoiceUrl": inv}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func preCheckout(b *gotgbot.Bot, ctx *ext.Context) error {
	_, err := ctx.PreCheckoutQuery.Answer(b, true, nil)
	if err != nil {
		return fmt.Errorf("failed to answer precheckout query: %w", err)
	}
	return nil
}

func paymentComplete(b *gotgbot.Bot, ctx *ext.Context) error {
	userID := ctx.EffectiveMessage.From.Id
	// Cộng số tiền đã thanh toán vào balance của user
	userBalances[userID] += 1 // Giả sử mỗi lần thanh toán là 1 Star
	_, err := ctx.EffectiveMessage.Reply(b, "Payment complete - in a real bot, this is where you would provision the product that has been paid for.", nil)
	if err != nil {
		return fmt.Errorf("failed to send payment complete message: %w", err)
	}
	return nil
}
func getBalanceHandler(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.URL.Query().Get("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	balance := userBalances[userID]
	response := map[string]interface{}{
		"balance": balance,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
