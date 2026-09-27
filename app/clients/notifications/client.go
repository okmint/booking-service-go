package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// ErrClientError используется для обозначения ошибок 4xx, которые не нужно ретраить.
var ErrClientError = errors.New("client error (4xx)")

// Client -- HTTP-клиент для взаимодействия с Notification Service.
type Client struct {
	baseURL    string
	httpClient *http.Client
	retryCfg   RetryConfig
	logger     *zap.Logger
}

// NewClient создаёт новый клиент для отправки уведомлений.
func NewClient(baseURL string, timeout time.Duration, maxRetries int, retryBaseDelay time.Duration, logger *zap.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryCfg: RetryConfig{
			MaxRetries: maxRetries,
			BaseDelay:  retryBaseDelay,
		},
		logger: logger,
	}
}

// SendNotification отправляет уведомление о смене статуса бронирования.
func (c *Client) SendNotification(ctx context.Context, req NotificationRequest) error {
	err := withRetry(ctx, c.retryCfg, "SendNotification", func() error {
		_, err := c.doJSON(ctx, http.MethodPost, "/api/notifications", req)
		return err
	})

	if err != nil {
		c.logger.Warn("Не удалось отправить уведомление, игнорируем ошибку",
			zap.Any("request", req),
			zap.Error(err),
		)
		return nil
	}

	return nil
}

// doJSON выполняет HTTP-запрос с JSON-телом и возвращает тело ответа.
func (c *Client) doJSON(ctx context.Context, method, path string, body any) ([]byte, error) {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("сериализация тела запроса: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("создание запроса: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("выполнение запроса %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("чтение ответа: %w", err)
	}

	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return nil, fmt.Errorf("%w: HTTP %d от Notification Service: %s", ErrClientError, resp.StatusCode, string(respBody))
	}

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("HTTP %d от Notification Service (%s %s): %s", resp.StatusCode, method, path, string(respBody))
	}

	return respBody, nil
}
