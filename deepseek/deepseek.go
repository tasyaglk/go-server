package deepseek

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	deepSeekBaseURL = "https://api.deepseek.com/v1/chat/completions"
	deepSeekAPIKey  = "sk-5f43682799c54539bed59121f9b02615"
)

type DeepSeekRequest struct {
	Model            string    `json:"model"`
	Messages         []Message `json:"messages"`
	Temperature      float64   `json:"temperature"`
	MaxTokens        int       `json:"max_tokens"`
	TopP             float64   `json:"top_p"`
	FrequencyPenalty float64   `json:"frequency_penalty"`
	PresencePenalty  float64   `json:"presence_penalty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type DeepSeekResponse struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message Message `json:"message"`
}

type DeepSeekError struct {
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

type DeepSeekManager struct {
	client *http.Client
}

func NewDeepSeekManager() *DeepSeekManager {
	return &DeepSeekManager{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (m *DeepSeekManager) sendRequest(prompt string) (string, error) {
	reqBody := DeepSeekRequest{
		Model:            "deepseek-chat",
		Messages:         []Message{{Role: "user", Content: prompt}},
		Temperature:      0.7,
		MaxTokens:        1000,
		TopP:             1.0,
		FrequencyPenalty: 0.0,
		PresencePenalty:  0.0,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", deepSeekBaseURL, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+deepSeekAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		var errorResp DeepSeekError
		if err := json.Unmarshal(bodyBytes, &errorResp); err == nil {
			return "", fmt.Errorf("API error: %s (code: %d)", errorResp.Error.Message, errorResp.Error.Code)
		}
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResponse DeepSeekResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}

	if len(apiResponse.Choices) == 0 || apiResponse.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("empty response content")
	}

	return apiResponse.Choices[0].Message.Content, nil
}

func (m *DeepSeekManager) ReformulateGoal(goal string) (string, error) {
	prompt := fmt.Sprintf(`
        Переформулируй следующую цель кратко и четко, сохраняя ее суть: "%s".
        Ответ должен быть не длиннее 10 слов, понятным и конкретным.
        Выведи только переформулированную цель, без дополнительных комментариев и без кавычек.
    `, goal)

	response, err := m.sendRequest(prompt)
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(response)
	return strings.Trim(trimmed, "\""), nil
}

func (m *DeepSeekManager) ValidateGoal(goal string) (bool, error) {
	prompt := fmt.Sprintf(`
        Оцени следующую цель на моральную и социальную приемлемость: "%s".
        Ответ Phillies только "true" или "false".
        Если цель нарушает моральные или социальные нормы (например, связана с насилием, дискриминацией, незаконной деятельностью), верни "false".
        Если цель нейтральна или позитивна (например, связана с обучением, саморазвитием, творчеством), верни "true".
    `, goal)

	response, err := m.sendRequest(prompt)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(response) == "true", nil
}

func (m *DeepSeekManager) ValidatePlanFeedback(feedback, goal string) (bool, error) {
	prompt := fmt.Sprintf(`
        Проверь, относится ли следующее пожелание к цели: "%s".
        Цель: "%s".
        Ответь только "true" или "false".
        Если пожелание связано с изменением плана для достижения цели (например, уточнение шагов, добавление деталей, изменение подхода), верни "true".
        Если пожелание не связано с целью (например, отвлеченная тема, не относящаяся к достижению цели), верни "false".
    `, feedback, goal)

	response, err := m.sendRequest(prompt)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(response) == "true", nil
}

func (m *DeepSeekManager) ValidateScheduleFeedback(feedback string) (bool, error) {
	prompt := fmt.Sprintf(`
        Проверь, относится ли следующее пожелание к расписанию: "%s".
        Ответь только "true" или "false".
        Если пожелание связано с изменением расписания (например, изменение времени, частоты, порядка задач), верни "true".
        Если пожелание не связано с расписанием (например, отвлеченная тема, не относящаяся к планированию времени), верни "false".
    `, feedback)

	response, err := m.sendRequest(prompt)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(response) == "true", nil
}

func (m *DeepSeekManager) GenerateSteps(goal, knowledge, feedback string) ([]string, error) {
	var prompt string
	if feedback != "" {
		prompt = fmt.Sprintf(`
            Пользователь хочет: %s
            Уже знает: %s
            Дополнительные пожелания: %s
            Сгенерируй четкие шаги для достижения цели с учетом всех пожеланий. Только пункты списка, без пояснений.
            Формат ответа: каждый пункт с новой строки без номеров
        `, goal, knowledge, feedback)
	} else {
		prompt = fmt.Sprintf(`
            Пользователь хочет: %s
            Уже знает: %s
            Сгенерируй четкие шаги для достижения цели. Только пункты списка, без пояснений.
            Формат ответа: каждый пункт с новой строки без номеров
        `, goal, knowledge)
	}

	response, err := m.sendRequest(prompt)
	if err != nil {
		return nil, err
	}

	steps := strings.Split(response, "\n")
	var result []string
	for _, step := range steps {
		trimmed := strings.TrimSpace(step)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result, nil
}

func (m *DeepSeekManager) GenerateSchedule(steps []string, availability, frequency, feedback string, busySlots [][2]time.Time) (string, error) {
	var busySlotsText strings.Builder
	if len(busySlots) == 0 {
		busySlotsText.WriteString("Нет занятых слотов")
	} else {
		for i, slot := range busySlots {
			start := slot[0].Format("02-01-2006 с 15:04 до")
			end := slot[1].Format("15:04")
			busySlotsText.WriteString(start + " " + end)
			if i < len(busySlots)-1 {
				busySlotsText.WriteString("\n")
			}
		}
	}

	currentDate := time.Now().Format("02-01-2006")
	var prompt string
	if feedback != "" {
		prompt = fmt.Sprintf(`
            Одобренные шаги для выполнения (не добавляй новые шаги, используй только эти):
            %s
            
            Доступность пользователя: %s
            Частота занятий: %s
            
            Занятые временные слоты (избегай их при планировании):
            %s
            
            Дополнительные пожелания к расписанию: %s
            
            Создай расписание в формате (это очень важно):
            Задача - ДД-ММ-ГГГГ в ЧЧ:мм
            
            Учитывай:
            - Используй ТОЛЬКО предоставленные шаги, не придумывай новые.
            - Минимальная длительность каждого задания должна быть 1 час.
            - Доступность и частоту, указанные пользователем.
            - Занятые временные слоты (не назначай задачи на эти периоды).
            - Реальные сроки выполнения задач.
            - Перерывы между задачами.
            - Текущая дата: %s.
            
            Выведи только расписание, без дополнительных комментариев.
        `, strings.Join(steps, "\n"), availability, frequency, busySlotsText.String(), feedback, currentDate)
	} else {
		prompt = fmt.Sprintf(`
            Одобренные шаги для выполнения (не добавляй новые шаги, используй только эти):
            %s
            
            Доступность пользователя: %s
            Частота занятий: %s
            
            Занятые временные слоты (избегай их при планировании):
            %s
            
            Создай расписание в формате (это очень важно):
            Задача - ДД-ММ-ГГГГ в ЧЧ:мм
            
            Учитывай:
            - Используй ТОЛЬКО предоставленные шаги, не придумывай новые.
            - Минимальная длительность каждого задания должна быть 1 час.
            - Доступность и частоту, указанные пользователем.
            - Занятые временные слоты (не назначай задачи на эти периоды).
            - Реальные сроки выполнения задач.
            - Перерывы между задачами.
            - Текущая дата: %s.
            
            Выведи только расписание, без дополнительных комментариев.
        `, strings.Join(steps, "\n"), availability, frequency, busySlotsText.String(), currentDate)
	}

	response, err := m.sendRequest(prompt)
	if err != nil {
		return "", err
	}

	return response, nil
}
