package jira

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/snonux/tasksamurai/internal/task"
)

// CommentPage is the first page of comments and its unseen remainder.
type CommentPage struct {
	Annotations []task.Annotation
	Remaining   int
}

type adfNode struct {
	Type    string         `json:"type"`
	Version int            `json:"version,omitempty"`
	Text    string         `json:"text,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []adfNode      `json:"content,omitempty"`
}

type comment struct {
	Created string  `json:"created"`
	Body    adfNode `json:"body"`
}

// AddComment posts an ADF comment and returns the server-created annotation.
func (c *Client) AddComment(ctx context.Context, key, text string) (task.Annotation, error) {
	path, err := issuePath(key)
	if err != nil {
		return task.Annotation{}, err
	}
	if strings.TrimSpace(text) == "" {
		return task.Annotation{}, errors.New("Jira: comment must not be empty")
	}
	var created comment
	if err := c.request(ctx, http.MethodPost, path+"/comment", map[string]any{"body": commentDocument(text)}, &created, false); err != nil {
		return task.Annotation{}, err
	}
	annotation := mapComment(created)
	if annotation.Entry == "" || created.Body.Type != "doc" {
		return task.Annotation{}, errors.New("Jira: invalid created comment response; comment may have been posted")
	}
	return annotation, nil
}

// FetchComments fetches only the first page and reports how many remain.
func (c *Client) FetchComments(ctx context.Context, key string) (CommentPage, error) {
	path, err := issuePath(key)
	if err != nil {
		return CommentPage{}, err
	}
	var page struct {
		Comments []comment `json:"comments"`
		Total    int       `json:"total"`
	}
	if err := c.request(ctx, http.MethodGet, path+"/comment?startAt=0&maxResults=50", nil, &page, true); err != nil {
		return CommentPage{}, err
	}
	if page.Comments == nil {
		return CommentPage{}, errors.New("Jira: comment response has no comments array")
	}
	result := CommentPage{Annotations: make([]task.Annotation, 0, len(page.Comments)), Remaining: max(0, page.Total-len(page.Comments))}
	for _, value := range page.Comments {
		result.Annotations = append(result.Annotations, mapComment(value))
	}
	return result, nil
}

func commentDocument(text string) adfNode {
	root := adfNode{Type: "doc", Version: 1}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		paragraph := adfNode{Type: "paragraph"}
		if line != "" {
			paragraph.Content = []adfNode{{Type: "text", Text: line}}
		}
		root.Content = append(root.Content, paragraph)
	}
	return root
}

func mapComment(value comment) task.Annotation {
	return task.Annotation{Entry: taskDate(value.Created), Description: strings.TrimSuffix(adfText(value.Body), "\n")}
}

func adfText(node adfNode) string {
	switch node.Type {
	case "text":
		return node.Text
	case "hardBreak":
		return "\n"
	case "mention":
		if text, ok := node.Attrs["text"].(string); ok {
			return text
		}
	case "emoji":
		if text, ok := node.Attrs["text"].(string); ok {
			return text
		}
	case "inlineCard":
		if text, ok := node.Attrs["url"].(string); ok {
			return text
		}
	}
	var text strings.Builder
	for _, child := range node.Content {
		text.WriteString(adfText(child))
	}
	if node.Type == "paragraph" || node.Type == "heading" || node.Type == "codeBlock" {
		text.WriteByte('\n')
	}
	return text.String()
}
