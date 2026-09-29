package types

import "context"

type FunctionSet struct {
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	Modified []string `json:"modified"`
	Muted    []string `json:"muted"`
}
type FileChange struct {
	Name       string   `json:"name"`
	Action     string   `json:"action"`
	LineCounts []string `json:"lineCounts"`
}
type Changes struct {
	Type      string       `json:"type"`
	Path      string       `json:"path"`
	Functions []FileChange `json:"functions"`
}

type CommitInfo struct {
	ID           string      `json:"id"`
	Message      string      `json:"message"`
	Timestamp    string      `json:"timestamp"`
	AlibiViewURL string      `json:"alibiViewUrl,omitempty"`
	GithubURL    string      `json:"githubUrl,omitempty"`
	Summary      SummaryData `json:"summary,omitempty"`
}
type Activity struct {
	ID          string       `json:"id"`
	Type        string       `json:"type"` // "commit" or "push"
	Timestamp   string       `json:"timestamp"`
	Commits     []CommitInfo `json:"commits"`
	FileChanges []Changes    `json:"changes"`
}

type TrackerFile struct {
	Date       string     `json:"date,omitempty"`
	Activities []Activity `json:"activities"`
}

type ConfigAI struct {
	OpenAI      string `yaml:"OpenAI" json:"OpenAI"`
	DeepSeek    string `yaml:"DeepSeek" json:"DeepSeek"`
	OpenRouter  string `yaml:"OpenRouter" json:"OpenRouter"`
	HuggingFace string `yaml:"HuggingFace" json:"HuggingFace"`
}

func (c ConfigAI) AsMap() map[string]string {
	return map[string]string{
		"openai":      c.OpenAI,
		"deepseek":    c.DeepSeek,
		"openrouter":  c.OpenRouter,
		"huggingface": c.HuggingFace,
	}
}

type Config struct {
	Project struct {
		Name   string `yaml:"name" json:"name"`
		Owner  string `yaml:"owner" json:"owner"`
		Source string `yaml:"source" json:"source"`
		Data   string `yaml:"data" json:"data"`
	} `yaml:"project" json:"project"`

	Tracking struct {
		Include []string `yaml:"include" json:"include"`
		Exclude []string `yaml:"exclude" json:"exclude"`
		Repo    string   `yaml:"repo" json:"repo"`
		Branch  string   `yaml:"branch" json:"branch"`
		Remote  string   `yaml:"remote" json:"remote"`
	} `yaml:"tracking" json:"tracking"`

	AIProviders ConfigAI `yaml:"AIProviders" json:"AIProviders"`
}

type ProjectData struct {
	Name        string      `json:"name"`
	Repo        string      `json:"repo"`
	LastUpdated string      `json:"lastUpdated"`
	AIProviders []string    `json:"AIProviders"`
	Data        TrackerFile `json:"data"`
	Path        string      `json:"path"`
	Source      string      `json:"source"`
}

type ProjectSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	LastUpdated string `json:"lastUpdated"`
}
type SummaryData struct {
	CommitID  string `json:"commitID"`
	Msg       string `json:"msg"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	TimeStamp string `json:"timestamp"`
	Content   string `json:"content"`
}

type CommitSummary struct {
	ProjectID string        `json:"projectID"`
	Data      []SummaryData `json:"data"`
}

type DownloadPDF struct {
	ProjectID  string     `json:"projectID"`
	Activities []Activity `json:"activities"`
	UseAI      bool       `json:"useAI"`
	Provider   string     `json:"provider"`
	Model      string     `json:"model"` // empty falls back to the provider default
	StartDate  string     `json:"startDate"`
	EndDate    string     `json:"endDate"`
	Source     string     `json:"source"`
	Directory  string     `json:"dir"`
}

type AIProvider struct {
	Name  string `json:"name"`
	Key   string `json:"key"`
	Model string `json:"model"`
}

// ModelInfo describes one model a provider can serve. Prices are normalised to
// US dollars per million tokens so the dashboard can compare providers directly,
// regardless of the unit each API reports in.
//
// PricingKnown is false for providers that publish no pricing (OpenAI,
// DeepSeek). Free is only meaningful when PricingKnown is true.
type ModelInfo struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Free          bool    `json:"free"`
	PricingKnown  bool    `json:"pricingKnown"`
	InputPer1M    float64 `json:"inputPer1M"`
	OutputPer1M   float64 `json:"outputPer1M"`
	ContextLength int     `json:"contextLength,omitempty"`
}

// ------- Types --------
type CommitData struct {
	ProjectID string   `json:"projectId"`
	Source    string   `json:"source"` // repo working tree path
	Dir       string   `json:"dir"`
	Hash      string   `json:"hash"`     // commit hash
	Message   string   `json:"message"`  // commit message
	Paths     []string `json:"paths"`    // files touched in the commit
	Provider  string   `json:"provider"` // LLM provider id
	Model     string   `json:"model"`    // empty falls back to the provider default
}

type Job struct {
	Path       string
	Diff       string
	Size       int // number of lines (diff size measure)
	CommitMsg  string
	Provider   AIProvider
	CommitHash string
	RepoDir    string
}

type SummaryResult struct {
	Path string `json:"path"`
	HTML string `json:"html"`
	Err  string `json:"err,omitempty"`
}

type SummaryEvent struct {
	Type string
	Data any
}

type SummaryProcessor func(ctx context.Context, data CommitData) <-chan SummaryEvent
