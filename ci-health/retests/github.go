package retests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func FetchMergedPRs(ctx context.Context, token, org, repo string, since time.Time) ([]MergedPR, error) {
	type ghPR struct {
		Number   int    `json:"number"`
		Title    string `json:"title"`
		MergedAt string `json:"merged_at"`
		Head     struct {
			SHA string `json:"sha"`
		} `json:"head"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	var allPRs []MergedPR
	for page := 1; ; page++ {
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=closed&sort=updated&direction=desc&per_page=100&page=%d", org, repo, page)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("github API: %w", err)
		}

		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fmt.Errorf("github API returned %d", resp.StatusCode)
		}

		var prs []ghPR
		if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("parsing github response: %w", err)
		}
		resp.Body.Close()

		if len(prs) == 0 {
			break
		}

		pastWindow := false
		for _, pr := range prs {
			if pr.MergedAt == "" {
				continue
			}
			mergedAt, err := time.Parse(time.RFC3339, pr.MergedAt)
			if err != nil {
				continue
			}
			if mergedAt.Before(since) {
				pastWindow = true
				break
			}
			allPRs = append(allPRs, MergedPR{
				Number:   pr.Number,
				Title:    pr.Title,
				Author:   pr.User.Login,
				MergedAt: mergedAt,
				HeadSHA:  pr.Head.SHA,
			})
		}

		if pastWindow || len(prs) < 100 {
			break
		}
	}

	return allPRs, nil
}
