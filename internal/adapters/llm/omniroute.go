package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"code-base-golang/internal/config"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/jsonutil"
	"code-base-golang/internal/services"
)

// OmniRouteLLM implements services.LLMProvider using an OpenAI-compatible /chat/completions endpoint.
type OmniRouteLLM struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

// NewOmniRoute creates a new OmniRoute OpenAI-compatible LLM adapter.
func NewOmniRoute(cfg config.Config, customClient ...*http.Client) *OmniRouteLLM {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.LLMBaseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:20128/v1"
	}

	model := strings.TrimSpace(cfg.LLMModel)
	if model == "" {
		model = "gpt-4o-mini"
	}

	client := &http.Client{Timeout: 3 * time.Minute}
	if len(customClient) > 0 && customClient[0] != nil {
		client = customClient[0]
	}

	return &OmniRouteLLM{
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(cfg.LLMAPIKey),
		model:      model,
		httpClient: client,
	}
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponseFormat struct {
	Type       string                 `json:"type"`
	JSONSchema *openAIJSONSchemaField `json:"json_schema,omitempty"`
}

type openAIJSONSchemaField struct {
	Name   string                 `json:"name"`
	Strict bool                   `json:"strict"`
	Schema map[string]interface{} `json:"schema"`
}

type openAIChatRequest struct {
	Model          string                `json:"model"`
	Messages       []openAIChatMessage   `json:"messages"`
	Temperature    *float64              `json:"temperature,omitempty"`
	MaxTokens      *int                  `json:"max_tokens,omitempty"`
	TopP           *float64              `json:"top_p,omitempty"`
	ResponseFormat *openAIResponseFormat `json:"response_format,omitempty"`
	Stream         bool                  `json:"stream,omitempty"`
}

type openAIChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
}

// GenerateStructured requests the LLM to populate a structured output guaranteed to match the given JSON schema.
func (o *OmniRouteLLM) GenerateStructured(ctx context.Context, systemPrompt string, userPrompt string, schema map[string]interface{}) (*dtos.StructuredResponse, error) {
	if schema != nil {
		schemaBytes, _ := json.Marshal(schema)
		schemaInstruction := fmt.Sprintf("\n\nCRITICAL REQUIREMENT: You MUST respond with ONLY a valid raw JSON object strictly conforming to the following JSON schema:\n%s\nDo NOT include any explanation or markdown formatting outside the JSON object.", string(schemaBytes))
		systemPrompt += schemaInstruction
	}

	var messages []openAIChatMessage
	if systemPrompt != "" {
		messages = append(messages, openAIChatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	messages = append(messages, openAIChatMessage{
		Role:    "user",
		Content: userPrompt,
	})

	respFormat := &openAIResponseFormat{
		Type: "json_object",
	}

	temp := 0.1
	reqPayload := openAIChatRequest{
		Model:          o.model,
		Messages:       messages,
		Temperature:    &temp,
		ResponseFormat: respFormat,
	}

	rawResp, err := o.executeChatRequest(ctx, reqPayload)
	if err != nil {
		return nil, err
	}

	if len(rawResp.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}

	cleanedJSON := jsonutil.CleanMarkdownJSON(rawResp.Choices[0].Message.Content)

	return &dtos.StructuredResponse{
		RawJSON: cleanedJSON,
		Usage: dtos.LLMUsage{
			PromptTokens:     rawResp.Usage.PromptTokens,
			CompletionTokens: rawResp.Usage.CompletionTokens,
			TotalTokens:      rawResp.Usage.TotalTokens,
		},
	}, nil
}

// GenerateChatResponse conducts a standard non-streaming multi-turn chat interaction.
func (o *OmniRouteLLM) GenerateChatResponse(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (*dtos.ChatResponse, error) {
	var chatMessages []openAIChatMessage
	if systemPrompt != "" {
		chatMessages = append(chatMessages, openAIChatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	for _, m := range messages {
		chatMessages = append(chatMessages, openAIChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	reqPayload := openAIChatRequest{
		Model:       o.model,
		Messages:    chatMessages,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		TopP:        opts.TopP,
	}

	rawResp, err := o.executeChatRequest(ctx, reqPayload)
	if err != nil {
		return nil, err
	}

	if len(rawResp.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}

	choice := rawResp.Choices[0]
	return &dtos.ChatResponse{
		Content:      choice.Message.Content,
		Role:         choice.Message.Role,
		FinishReason: choice.FinishReason,
		Usage: dtos.LLMUsage{
			PromptTokens:     rawResp.Usage.PromptTokens,
			CompletionTokens: rawResp.Usage.CompletionTokens,
			TotalTokens:      rawResp.Usage.TotalTokens,
		},
	}, nil
}

// StreamChatResponse streams completion chunks using Server-Sent Events (SSE).
func (o *OmniRouteLLM) StreamChatResponse(ctx context.Context, systemPrompt string, messages []dtos.ChatMessageInput, opts dtos.ChatOptions) (<-chan dtos.StreamChunk, error) {
	var chatMessages []openAIChatMessage
	if systemPrompt != "" {
		chatMessages = append(chatMessages, openAIChatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	for _, m := range messages {
		chatMessages = append(chatMessages, openAIChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	reqPayload := openAIChatRequest{
		Model:       o.model,
		Messages:    chatMessages,
		Temperature: opts.Temperature,
		MaxTokens:   opts.MaxTokens,
		TopP:        opts.TopP,
		Stream:      true,
	}

	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal streaming request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", o.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm stream request error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		errBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("llm stream returned status %d: %s", resp.StatusCode, string(errBytes))
	}

	outChan := make(chan dtos.StreamChunk, 20)

	go func() {
		defer resp.Body.Close()
		defer close(outChan)

		reader := bufio.NewReader(resp.Body)

		for {
			select {
			case <-ctx.Done():
				outChan <- dtos.StreamChunk{Err: ctx.Err()}
				return
			default:
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					outChan <- dtos.StreamChunk{Err: fmt.Errorf("error reading stream line: %w", err)}
				}
				return
			}

			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}

			if !strings.HasPrefix(trimmed, "data: ") {
				continue
			}

			payload := strings.TrimPrefix(trimmed, "data: ")
			if payload == "[DONE]" {
				return
			}

			var chunk openAIStreamChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				outChan <- dtos.StreamChunk{Err: fmt.Errorf("failed to parse stream chunk json: %w", err)}
				return
			}

			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				finishReason := chunk.Choices[0].FinishReason
				if delta.Content != "" || finishReason != "" {
					outChan <- dtos.StreamChunk{
						Content:      delta.Content,
						FinishReason: finishReason,
					}
				}
			}
		}
	}()

	return outChan, nil
}

func (o *OmniRouteLLM) executeChatRequest(ctx context.Context, payload openAIChatRequest) (*openAIChatResponse, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request payload: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", o.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm api returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	trimmed := bytes.TrimSpace(respBytes)
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		return parseSSEChatResponse(trimmed)
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		// Fallback: check if response payload is an SSE event stream
		if sseResp, sseErr := parseSSEChatResponse(trimmed); sseErr == nil && len(sseResp.Choices) > 0 && sseResp.Choices[0].Message.Content != "" {
			return sseResp, nil
		}
		return nil, fmt.Errorf("failed to unmarshal chat response: %w", err)
	}

	return &chatResp, nil
}

// parseSSEChatResponse reconstructs a full chat completion response from an SSE event stream payload.
func parseSSEChatResponse(respBytes []byte) (*openAIChatResponse, error) {
	reader := bufio.NewReader(bytes.NewReader(respBytes))
	var contentBuilder strings.Builder
	var finishReason string
	var usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed reading sse stream: %w", err)
		}

		trimmed := strings.TrimSpace(line)
		if trimmed != "" && strings.HasPrefix(trimmed, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if payload == "[DONE]" {
				break
			}

			var chunk struct {
				ID      string `json:"id"`
				Choices []struct {
					Index int `json:"index"`
					Delta struct {
						Role    string `json:"role,omitempty"`
						Content string `json:"content,omitempty"`
					} `json:"delta"`
					Message struct {
						Role    string `json:"role,omitempty"`
						Content string `json:"content,omitempty"`
					} `json:"message"`
					FinishReason string `json:"finish_reason,omitempty"`
				} `json:"choices"`
				Usage struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage"`
			}

			if err := json.Unmarshal([]byte(payload), &chunk); err == nil {
				if chunk.Usage.TotalTokens > 0 {
					usage = chunk.Usage
				}
				if len(chunk.Choices) > 0 {
					choice := chunk.Choices[0]
					if choice.Delta.Content != "" {
						contentBuilder.WriteString(choice.Delta.Content)
					} else if choice.Message.Content != "" {
						contentBuilder.WriteString(choice.Message.Content)
					}
					if choice.FinishReason != "" {
						finishReason = choice.FinishReason
					}
				}
			}
		}

		if err == io.EOF {
			break
		}
	}

	if contentBuilder.Len() == 0 {
		return nil, fmt.Errorf("empty content in sse stream response")
	}

	if finishReason == "" {
		finishReason = "stop"
	}

	chatResp := &openAIChatResponse{
		Choices: []struct {
			Index   int `json:"index"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Index: 0,
				Message: struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{
					Role:    "assistant",
					Content: contentBuilder.String(),
				},
				FinishReason: finishReason,
			},
		},
	}
	chatResp.Usage.PromptTokens = usage.PromptTokens
	chatResp.Usage.CompletionTokens = usage.CompletionTokens
	chatResp.Usage.TotalTokens = usage.TotalTokens

	return chatResp, nil
}

// Ensure OmniRouteLLM satisfies services.LLMProvider at compile time.
var _ services.LLMProvider = (*OmniRouteLLM)(nil)
