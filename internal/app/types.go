package app

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type Project struct {
	ImportKey   string `json:"importKey,omitempty"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
}
type Role struct {
	MigratedSkillID string   `json:"migratedSkillId,omitempty"`
	KnowledgeIDs    []string `json:"knowledgeIds,omitempty"`
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Instructions    string   `json:"instructions"`
	Kind            string   `json:"kind"`
	SkillIDs        []string `json:"skillIds"`
	Builtin         bool     `json:"builtin"`
}
type Model struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Family   string `json:"family"`
	BaseURL  string `json:"baseUrl"`
	ModelID  string `json:"modelId"`
	Secret   string `json:"secret,omitempty"`
	HasKey   bool   `json:"hasKey"`
	TestedAt string `json:"testedAt,omitempty"`
}
type Extension struct {
	SourceRoleID string   `json:"sourceRoleId,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	KnowledgeIDs []string `json:"knowledgeIds,omitempty"`
	Builtin      bool     `json:"builtin,omitempty"`
	Category     string   `json:"category,omitempty"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Description  string   `json:"description"`
	Content      string   `json:"content,omitempty"`
	URL          string   `json:"url,omitempty"`
	Enabled      bool     `json:"enabled"`
	Version      string   `json:"version"`
	Status       string   `json:"status"`
}
type Asset struct {
	RequireRunConfirmation bool   `json:"requireRunConfirmation,omitempty"`
	SourceName             string `json:"sourceName,omitempty"`
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	Kind                   string `json:"kind"`
	Size                   int64  `json:"size"`
	ProjectID              string `json:"projectId"`
	TaskID                 string `json:"taskId,omitempty"`
	ParentID               string `json:"parentId,omitempty"`
	CreatedAt              string `json:"createdAt"`
	Path                   string `json:"path,omitempty"`
	Preview                string `json:"preview,omitempty"`
	Validation             string `json:"validation"`
}
type Step struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	At     string `json:"at"`
}
type Task struct {
	ScheduleID     string      `json:"scheduleId,omitempty"`
	ScheduleRunID  string      `json:"scheduleRunId,omitempty"`
	ConversationID string      `json:"conversationId,omitempty"`
	AgentGroupID   string      `json:"agentGroupId,omitempty"`
	AgentMemberID  string      `json:"agentMemberId,omitempty"`
	UsedSkillIDs   []string    `json:"usedSkillIds,omitempty"`
	WorkflowRunID  string      `json:"workflowRunId,omitempty"`
	WorkflowNodeID string      `json:"workflowNodeId,omitempty"`
	Config         *TaskConfig `json:"configSnapshot,omitempty"`
	ID             string      `json:"id"`
	ProjectID      string      `json:"projectId"`
	ParentID       string      `json:"parentId,omitempty"`
	Title          string      `json:"title"`
	Prompt         string      `json:"prompt"`
	Kind           string      `json:"kind"`
	RoleID         string      `json:"roleId"`
	ModelID        string      `json:"modelId"`
	ModelName      string      `json:"modelName"`
	Status         string      `json:"status"`
	InputIDs       []string    `json:"inputIds"`
	OutputIDs      []string    `json:"outputIds"`
	Steps          []Step      `json:"steps"`
	Result         string      `json:"result"`
	Error          string      `json:"error,omitempty"`
	CreatedAt      string      `json:"createdAt"`
	UpdatedAt      string      `json:"updatedAt"`
	Archived       bool        `json:"archived"`
}

// TaskConfig contains no credential material. Credentials stay in the model
// store (DPAPI); the running worker receives its selected secret only in memory.
type TaskConfig struct {
	AgentStage          string           `json:"agentStage,omitempty"`
	AgentCandidateIDs   []string         `json:"agentCandidateIds,omitempty"`
	OwnedPaths          []string         `json:"ownedPaths,omitempty"`
	ProjectInstructions string           `json:"projectInstructions,omitempty"`
	MemoryRefs          []MemoryRef      `json:"memoryRefs,omitempty"`
	LazySkills          bool             `json:"lazySkills,omitempty"`
	ConversationContext string           `json:"conversationContext,omitempty"`
	KnowledgeIDs        []string         `json:"knowledgeIds,omitempty"`
	KnowledgeRefs       []KnowledgeHit   `json:"knowledgeRefs,omitempty"`
	SchemaVersion       int              `json:"schemaVersion"`
	CapturedAt          string           `json:"capturedAt"`
	Executor            string           `json:"executor"`
	Mode                string           `json:"mode"`
	OutputKind          string           `json:"outputKind"`
	AllowRemote         bool             `json:"allowRemote"`
	Role                Role             `json:"role"`
	RoleHash            string           `json:"roleHash"`
	Model               *TaskModelConfig `json:"model,omitempty"`
	Extensions          []TaskExtension  `json:"extensions"`
}

type TaskModelConfig struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Family        string `json:"family"`
	BaseURL       string `json:"baseUrl"`
	ModelID       string `json:"modelId"`
	CredentialRef string `json:"credentialRef,omitempty"`
}

type TaskExtension struct {
	Extension
	ContentHash string `json:"contentHash"`
}
type State struct {
	StartupImport      *ProjectImportResult `json:"startupImport,omitempty"`
	Memories           []Memory             `json:"memories"`
	Schedules          []Schedule           `json:"schedules"`
	ScheduleRuns       []ScheduleRun        `json:"scheduleRuns"`
	Directories        []DirectoryGrant     `json:"directories"`
	Notifications      []Notification       `json:"notifications"`
	AgentGroups        []AgentGroup         `json:"agentGroups"`
	KnowledgeBases     []KnowledgeBase      `json:"knowledgeBases"`
	KnowledgeDocuments []KnowledgeDocument  `json:"knowledgeDocuments"`
	Workflows          []WorkflowDefinition `json:"workflows"`
	WorkflowRuns       []WorkflowRun        `json:"workflowRuns"`
	Projects           []Project            `json:"projects"`
	Roles              []Role               `json:"roles"`
	Models             []Model              `json:"models"`
	Extensions         []Extension          `json:"extensions"`
	Assets             []Asset              `json:"assets"`
	Tasks              []Task               `json:"tasks"`
}
type Section struct {
	Heading string `json:"heading"`
	Body    string `json:"body"`
}
type Deliverable struct {
	Title    string    `json:"title"`
	Summary  string    `json:"summary"`
	Sections []Section `json:"sections"`
	Headers  []string  `json:"headers"`
	Rows     [][]any   `json:"rows"`
}

func id() string {
	b := make([]byte, 12)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func now() string { return time.Now().UTC().Format(time.RFC3339) }
func defaults() State {
	s := State{Projects: []Project{{ID: "inbox", Name: "我的工作空间", Description: "日常办公与临时任务", CreatedAt: now()}}, Models: []Model{}, Extensions: []Extension{}, Assets: []Asset{}, Tasks: []Task{}}
	s.AgentGroups = []AgentGroup{}
	s.Memories = []Memory{}
	s.Schedules = []Schedule{}
	s.ScheduleRuns = []ScheduleRun{}
	s.Directories = []DirectoryGrant{}
	s.Notifications = []Notification{}
	s.KnowledgeBases = []KnowledgeBase{}
	s.KnowledgeDocuments = []KnowledgeDocument{}
	s.Workflows = []WorkflowDefinition{}
	s.WorkflowRuns = []WorkflowRun{}
	for _, r := range []struct{ name, desc, kind, instruction string }{
		{"通用办公助手", "梳理资料，完成日常办公任务", "auto", "依据用户材料完成任务，事实与建议分开，缺失信息不编造。"},
		{"演示文稿助手", "把材料整理成清晰的演示文稿", "pptx", "按观点组织页面，每页一个重点，使用简洁的标题和正文。"},
		{"文档编辑", "撰写报告、方案与会议纪要", "docx", "结构清晰，保留输入中的人名、日期与关键数字，未知事实标为待补充。"},
		{"表格分析师", "清洗表格，汇总并核对数据", "xlsx", "保持数据准确，区分空值与零，不编造数据，保留文本编号的前导零。"},
		{"PDF 助手", "提取文字，合并和拆分页面", "pdf", "按照原文提取内容并标明页码。"},
		{"文件整理员", "按类型分类打包上传的文件", "organize", "分类保存输入文件，不修改原件。"},
	} {
		s.Roles = append(s.Roles, Role{ID: r.kind, Name: r.name, Description: r.desc, Kind: r.kind, Instructions: r.instruction, Builtin: true, SkillIDs: []string{}})
	}
	return s
}
