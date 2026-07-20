package retests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

const prowBaseURL = "https://prow.ci.openshift.org"

func FetchPRHistory(ctx context.Context, org, repo string, prNumber int) (*ProwPRHistory, error) {
	url := fmt.Sprintf("%s/pr-history/?org=%s&repo=%s&pr=%d", prowBaseURL, org, repo, prNumber)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching pr-history for PR %d: %w", prNumber, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("pr-history for PR %d returned %d", prNumber, resp.StatusCode)
	}

	return parsePRHistory(resp.Body)
}

func parsePRHistory(r io.Reader) (*ProwPRHistory, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parsing HTML: %w", err)
	}

	table := findNode(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "table" && hasAttrValue(n, "id", "history-table")
	})
	if table == nil {
		return nil, fmt.Errorf("history-table not found")
	}

	result := &ProwPRHistory{}

	thead := findNode(table, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "thead"
	})
	if thead != nil {
		result.Commits = parseCommitHeaders(thead)
	}

	tbody := findNode(table, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "tbody"
	})
	if tbody != nil {
		result.Jobs = parseJobRows(tbody, result.Commits)
	}

	return result, nil
}

func parseCommitHeaders(thead *html.Node) []CommitColumn {
	var commits []CommitColumn
	tr := findNode(thead, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "tr"
	})
	if tr == nil {
		return nil
	}

	for th := tr.FirstChild; th != nil; th = th.NextSibling {
		if th.Type != html.ElementNode || th.Data != "th" {
			continue
		}
		colspan := attrVal(th, "colspan")
		if colspan == "" {
			continue
		}
		var cs int
		fmt.Sscanf(colspan, "%d", &cs)
		if cs == 0 {
			continue
		}

		sha := ""
		a := findNode(th, func(n *html.Node) bool {
			return n.Type == html.ElementNode && n.Data == "a"
		})
		if a != nil {
			sha = textContent(a)
		} else {
			span := findNode(th, func(n *html.Node) bool {
				return n.Type == html.ElementNode && n.Data == "span"
			})
			if span != nil {
				sha = textContent(span)
			}
		}

		commits = append(commits, CommitColumn{SHA: strings.TrimSpace(sha), Colspan: cs})
	}
	return commits
}

func parseJobRows(tbody *html.Node, commits []CommitColumn) []ProwJobHistory {
	colRanges := buildColRanges(commits)

	var jobs []ProwJobHistory
	for tr := tbody.FirstChild; tr != nil; tr = tr.NextSibling {
		if tr.Type != html.ElementNode || tr.Data != "tr" {
			continue
		}

		job := ProwJobHistory{}
		colIdx := 0
		first := true

		for td := tr.FirstChild; td != nil; td = td.NextSibling {
			if td.Type != html.ElementNode || td.Data != "td" {
				continue
			}

			if first {
				first = false
				a := findNode(td, func(n *html.Node) bool {
					return n.Type == html.ElementNode && n.Data == "a"
				})
				if a != nil {
					job.Name = textContent(a)
				}
				continue
			}

			a := findNode(td, func(n *html.Node) bool {
				return n.Type == html.ElementNode && n.Data == "a"
			})
			if a != nil {
				status := classToStatus(td)
				commitSHA := commitForCol(colIdx, colRanges)
				job.Runs = append(job.Runs, ProwJobRun{
					ID:        textContent(a),
					Status:    status,
					CommitSHA: commitSHA,
				})
			}
			colIdx++
		}

		if job.Name != "" {
			jobs = append(jobs, job)
		}
	}
	return jobs
}

type colRange struct {
	start int
	end   int
	sha   string
}

func buildColRanges(commits []CommitColumn) []colRange {
	var ranges []colRange
	pos := 0
	for _, c := range commits {
		ranges = append(ranges, colRange{start: pos, end: pos + c.Colspan, sha: c.SHA})
		pos += c.Colspan
	}
	return ranges
}

func commitForCol(col int, ranges []colRange) string {
	for _, r := range ranges {
		if col >= r.start && col < r.end {
			return r.sha
		}
	}
	return "unknown"
}

func classToStatus(n *html.Node) RunStatus {
	cls := attrVal(n, "class")
	switch {
	case strings.Contains(cls, "run-success"):
		return RunSuccess
	case strings.Contains(cls, "run-failure"):
		return RunFailure
	case strings.Contains(cls, "run-aborted"):
		return RunAborted
	case strings.Contains(cls, "run-pending"):
		return RunPending
	default:
		return ""
	}
}

func findNode(n *html.Node, match func(*html.Node) bool) *html.Node {
	if match(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findNode(c, match); found != nil {
			return found
		}
	}
	return nil
}

func hasAttrValue(n *html.Node, key, val string) bool {
	for _, a := range n.Attr {
		if a.Key == key && a.Val == val {
			return true
		}
	}
	return false
}

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(textContent(c))
	}
	return sb.String()
}
