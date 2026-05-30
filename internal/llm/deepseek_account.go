package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// BalanceInfo holds one currency balance entry from the DeepSeek
// /user/balance endpoint.
type BalanceInfo struct {
	Currency        string `json:"currency"`
	TotalBalance    string `json:"total_balance"`
	GrantedBalance  string `json:"granted_balance"`
	ToppedUpBalance string `json:"topped_up_balance"`
}

// UserBalance is the decoded response from GET /user/balance.
type UserBalance struct {
	IsAvailable  bool          `json:"is_available"`
	BalanceInfos []BalanceInfo `json:"balance_infos"`
}

// deepSeekAPIRoot converts c.BaseURL to the DeepSeek API root by
// trimming trailing slashes and removing exactly one trailing /v1
// segment when present.
func deepSeekAPIRoot(baseURL string) string {
	s := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(s, "/v1") {
		s = s[:len(s)-3]
	}
	return s
}

// GetUserBalance fetches the account balance from the DeepSeek
// /user/balance endpoint. Only meaningful for DeepSeek providers.
func (c *Client) GetUserBalance(ctx context.Context) (UserBalance, error) {
	root := deepSeekAPIRoot(c.BaseURL)
	url := root + "/user/balance"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return UserBalance{}, fmt.Errorf("create balance request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return UserBalance{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return UserBalance{}, &APIError{Status: resp.StatusCode, Body: string(buf)}
	}

	var ub UserBalance
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&ub); err != nil {
		return UserBalance{}, fmt.Errorf("decode balance response: %w", err)
	}
	return ub, nil
}
