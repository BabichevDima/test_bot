package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"

	// "strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Структуры данных Wildberries (оставляем из предыдущего кода)
type SearchResponse struct {
	Products []Product `json:"products"`
	Total    int       `json:"total"`
}

type Product struct {
	Id           int     `json:"id"`
	Brand        string  `json:"brand"`
	Name         string  `json:"name"`
	ReviewRating float32 `json:"reviewRating"`
	Feedbacks    int     `json:"feedbacks"`
	Supplier     string  `json:"supplier"`
	Sizes        []Size  `json:"sizes"`
}

type Size struct {
	Price struct {
		Basic     int `json:"basic"`
		Product   int `json:"product"`
		Logistics int `json:"logistics"`
	} `json:"price"`
}

// Конфигурация
const (
	TelegramToken = "8273048786:AAHK9EPK_edyYR3ldNVPNFCTF7og4Xs8BKw" // Замените на ваш токен
	WBAPIURL      = "https://recom.wb.ru/recom/sng/common/v8/search?ab_testing=false&appType=1&curr=byn&dest=-59202&hide_dtype=10;13;14&lang=ru&page=1&query=%s&resultset=catalog&spp=30&suppressSpellcheck=false"
)

func main() {
	// Инициализация бота
	bot, err := tgbotapi.NewBotAPI(TelegramToken)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Настройка обновлений
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	// Обработка входящих сообщений
	for update := range updates {
		if update.Message == nil {
			continue
		}

		// Обрабатываем только текстовые сообщения
		if !update.Message.IsCommand() && update.Message.Text != "" {
			go handleMessage(bot, update.Message)
		}
	}
}

func handleMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {

	log.Println("*****************************************")
	log.Println(bot)
	log.Println(message.Text)
	log.Println("*****************************************")

	// Извлекаем артикул из ссылки Wildberries
	nmId, err := extractNmIdFromURL(message.Text)
	if err != nil {
		sendMessage(bot, message.Chat.ID, "❌ Неверная ссылка Wildberries. Отправьте корректную ссылку на товар.")
		sendMessage(bot, message.Chat.ID, "🦾 Этот бот помогает находить похожие товары с самой минимальной ценой. Отправьте корректную ссылку на товар.")
		return
	}

	// Отправляем сообщение о начале поиска
	msg := tgbotapi.NewMessage(message.Chat.ID, "🔍 Ищу похожие товары...")
	bot.Send(msg)

	// Ищем похожие товары
	cheapestProduct, err := findCheapestSimilarProduct(nmId)
	if err != nil {
		sendMessage(bot, message.Chat.ID, "❌ Ошибка при поиске товаров: "+err.Error())
		return
	}

	if cheapestProduct == nil {
		sendMessage(bot, message.Chat.ID, "😔 Не удалось найти похожие товары.")
		return
	}

	// Формируем ответ с информацией о товаре
	response := formatProductResponse(cheapestProduct)
	sendMessage(bot, message.Chat.ID, response)
}

// Функция для извлечения артикула из URL Wildberries
func extractNmIdFromURL(url string) (string, error) {
	// Регулярное выражение для поиска артикула в URL Wildberries
	re := regexp.MustCompile(`wildberries\.(by|ru|kz|com)/catalog/(\d+)/`)
	matches := re.FindStringSubmatch(url)

	if len(matches) < 3 {
		return "", fmt.Errorf("артикул не найден в ссылке")
	}

	return matches[2], nil
}

// Функция для поиска самого дешевого похожего товара
func findCheapestSimilarProduct(nmId string) (*Product, error) {
	// Формируем URL запроса
	encodedQuery := url.QueryEscape("похожие " + nmId)
	apiURL := fmt.Sprintf(WBAPIURL, encodedQuery)

	// Создаем HTTP-клиент
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	// Добавляем заголовки
	req.Header.Set("User-Agent", "Mozilla/5.0 (PriceTrackerBot/1.0)")
	req.Header.Set("Accept", "application/json")

	// Выполняем запрос
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API вернуло ошибку: %s", resp.Status)
	}

	// Парсим ответ
	var searchResult SearchResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&searchResult); err != nil {
		return nil, err
	}

	// Ищем самый дешевый товар
	var cheapestProduct *Product
	cheapestPrice := -1

	for i := range searchResult.Products {
		product := &searchResult.Products[i]
		if len(product.Sizes) == 0 {
			continue
		}

		// Расчет итоговой цены
		size := product.Sizes[0]
		finalPrice := size.Price.Product + size.Price.Logistics

		if cheapestPrice == -1 || finalPrice < cheapestPrice {
			cheapestPrice = finalPrice
			cheapestProduct = product
		}
	}

	return cheapestProduct, nil
}

// Функция для форматирования ответа с информацией о товаре
func formatProductResponse(product *Product) string {
	if product == nil || len(product.Sizes) == 0 {
		return "Информация о товаре недоступна"
	}

	size := product.Sizes[0]
	finalPrice := float64(size.Price.Product+size.Price.Logistics) / 100
	// productPrice := float64(size.Price.Product) / 100 + float64(size.Price.Logistics) / 100
	// logisticsPrice := float64(size.Price.Logistics) / 100

	var builder strings.Builder

	builder.WriteString("🎯 *Самый дешевый похожий товар:*\n\n")
	builder.WriteString(fmt.Sprintf("🏷️ *Бренд:* %s\n", product.Brand))
	builder.WriteString(fmt.Sprintf("📦 *Название:* %s\n", product.Name))
	builder.WriteString(fmt.Sprintf("⭐ *Рейтинг:* %.1f/5\n", product.ReviewRating))
	builder.WriteString(fmt.Sprintf("💬 *Отзывов:* %d\n", product.Feedbacks))
	builder.WriteString(fmt.Sprintf("🏪 *Продавец:* %s\n", product.Supplier))
	builder.WriteString("\n💵 *Цена:*\n")
	// builder.WriteString(fmt.Sprintf("   Товар: %.2f руб.\n", productPrice))
	// builder.WriteString(fmt.Sprintf("   Доставка: %.2f руб.\n", logisticsPrice))
	builder.WriteString(fmt.Sprintf("   🎯 *Итого: %.2f руб.*\n", finalPrice))
	builder.WriteString("\n🔗 *Ссылка:*\n")
	builder.WriteString(fmt.Sprintf("https://www.wildberries.by/catalog/%d/detail.aspx", product.Id))

	return builder.String()
}

// Функция для отправки сообщения
func sendMessage(bot *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown" // Для форматирования текста
	bot.Send(msg)
}
