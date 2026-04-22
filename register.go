package pingback

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
)

func (p *Pingback) register() {
	funcs := make([]registerFunc, 0, len(p.functions))
	for _, f := range p.functions {
		rf := registerFunc{
			Name: f.name,
			Type: f.funcType,
			Options: registerOptions{
				Retries:     f.options.retries,
				Timeout:     f.options.timeout,
				Concurrency: f.options.concurrency,
			},
		}
		if f.funcType == "cron" {
			rf.Schedule = f.schedule
		}
		funcs = append(funcs, rf)
	}

	payload := registerPayload{
		Functions:   funcs,
		EndpointURL: p.opts.baseURL,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[pingback] failed to marshal registration payload: %v", err)
		return
	}

	url := p.opts.platformURL + "/api/v1/register"
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		log.Printf("[pingback] failed to create registration request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[pingback] registration failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		log.Printf("[pingback] registration returned status %d", resp.StatusCode)
		return
	}

	var result struct {
		Jobs []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"jobs"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	log.Printf("[pingback] registered %d function(s) with platform", len(result.Jobs))
}
