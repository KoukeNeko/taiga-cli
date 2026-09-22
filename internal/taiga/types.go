package taiga

import "encoding/json"

// Tag is a project tag as Taiga's API returns it on a work item: a
// [name, color-or-null] pair, not a flat string. Requests still send tag
// names as a flat []string; the API assigns/keeps colors on its own.
type Tag struct {
	Name  string
	Color *string
}

func (t *Tag) UnmarshalJSON(data []byte) error {
	var pair [2]*string
	if err := json.Unmarshal(data, &pair); err != nil {
		return err
	}
	var name string
	if pair[0] != nil {
		name = *pair[0]
	}
	*t = Tag{Name: name, Color: pair[1]}
	return nil
}

func (t Tag) MarshalJSON() ([]byte, error) {
	return json.Marshal([2]*string{&t.Name, t.Color})
}

type Page struct {
	Number int `json:"number"`
	Size   int `json:"size"`
	Total  int `json:"total"`
	Next   int `json:"next,omitempty"`
	Prev   int `json:"prev,omitempty"`
}

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	FullName string `json:"full_name_display,omitempty"`
	Email    string `json:"email,omitempty"`
}

type AuthResponse struct {
	AuthToken    string `json:"auth_token"`
	RefreshToken string `json:"refresh"`
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	FullName     string `json:"full_name_display"`
}

type Project struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Slug               string `json:"slug"`
	Description        string `json:"description,omitempty"`
	CreationTemplate   *int64 `json:"creation_template,omitempty"`
	IsPrivate          bool   `json:"is_private"`
	IsArchived         bool   `json:"is_archived,omitempty"`
	ArchivedCode       string `json:"archived_code,omitempty"`
	IsEpicsActivated   bool   `json:"is_epics_activated"`
	IsBacklogActivated bool   `json:"is_backlog_activated"`
	IsKanbanActivated  bool   `json:"is_kanban_activated"`
	IsWikiActivated    bool   `json:"is_wiki_activated"`
	IsIssuesActivated  bool   `json:"is_issues_activated"`
	CreatedDate        string `json:"created_date,omitempty"`
	ModifiedDate       string `json:"modified_date,omitempty"`
}

type ProjectTemplate struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	Order       int    `json:"order"`
}

type CreateProjectRequest struct {
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	CreationTemplate int64  `json:"creation_template"`
	IsPrivate        bool   `json:"is_private"`
}

type UpdateProjectRequest struct {
	Name               *string `json:"name,omitempty"`
	Description        *string `json:"description,omitempty"`
	IsPrivate          *bool   `json:"is_private,omitempty"`
	IsEpicsActivated   *bool   `json:"is_epics_activated,omitempty"`
	IsBacklogActivated *bool   `json:"is_backlog_activated,omitempty"`
	IsKanbanActivated  *bool   `json:"is_kanban_activated,omitempty"`
	IsWikiActivated    *bool   `json:"is_wiki_activated,omitempty"`
	IsIssuesActivated  *bool   `json:"is_issues_activated,omitempty"`
}

type Membership struct {
	ID             int64      `json:"id"`
	User           *int64     `json:"user,omitempty"`
	Project        int64      `json:"project"`
	Role           int64      `json:"role"`
	RoleName       string     `json:"role_name,omitempty"`
	IsAdmin        bool       `json:"is_admin"`
	IsOwner        bool       `json:"is_owner"`
	Email          string     `json:"email,omitempty"`
	UserEmail      string     `json:"user_email,omitempty"`
	FullName       string     `json:"full_name,omitempty"`
	IsUserActive   bool       `json:"is_user_active"`
	InvitedBy      *ExtraInfo `json:"invited_by,omitempty"`
	CreatedAt      string     `json:"created_at,omitempty"`
	InvitationText string     `json:"invitation_extra_text,omitempty"`
}

type CreateMembershipRequest struct {
	Username       string `json:"username"`
	Project        int64  `json:"project"`
	Role           int64  `json:"role"`
	IsAdmin        bool   `json:"is_admin"`
	InvitationText string `json:"invitation_extra_text,omitempty"`
}

type UpdateMembershipRequest struct {
	Role    *int64 `json:"role,omitempty"`
	IsAdmin *bool  `json:"is_admin,omitempty"`
}

type Role struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Slug         string   `json:"slug"`
	Project      int64    `json:"project"`
	Order        int      `json:"order"`
	Computable   bool     `json:"computable"`
	Permissions  []string `json:"permissions,omitempty"`
	MembersCount int      `json:"members_count"`
}

type CreateRoleRequest struct {
	Name        string   `json:"name"`
	Project     int64    `json:"project"`
	Computable  bool     `json:"computable"`
	Permissions []string `json:"permissions,omitempty"`
	Order       int      `json:"order,omitempty"`
}

type UpdateRoleRequest struct {
	Name        *string   `json:"name,omitempty"`
	Computable  *bool     `json:"computable,omitempty"`
	Permissions *[]string `json:"permissions,omitempty"`
	Order       *int      `json:"order,omitempty"`
}

type Webhook struct {
	ID          int64  `json:"id"`
	Project     int64  `json:"project"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Key         string `json:"key,omitempty"`
	LogsCounter int    `json:"logs_counter"`
}

type CreateWebhookRequest struct {
	Project int64  `json:"project"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Key     string `json:"key"`
}

type UpdateWebhookRequest struct {
	Name *string `json:"name,omitempty"`
	URL  *string `json:"url,omitempty"`
	Key  *string `json:"key,omitempty"`
}

type WebhookLog struct {
	ID              int64          `json:"id"`
	Webhook         int64          `json:"webhook"`
	URL             string         `json:"url"`
	Status          int            `json:"status"`
	RequestData     map[string]any `json:"request_data,omitempty"`
	ResponseData    string         `json:"response_data,omitempty"`
	ResponseHeaders map[string]any `json:"response_headers,omitempty"`
	Duration        float64        `json:"duration"`
	Created         string         `json:"created,omitempty"`
}

type CustomField struct {
	ID           int64          `json:"id"`
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	Type         string         `json:"type"`
	Order        int64          `json:"order"`
	Project      int64          `json:"project"`
	Extra        map[string]any `json:"extra,omitempty"`
	CreatedDate  string         `json:"created_date,omitempty"`
	ModifiedDate string         `json:"modified_date,omitempty"`
}

type CreateCustomFieldRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Type        string         `json:"type"`
	Order       int64          `json:"order,omitempty"`
	Project     int64          `json:"project"`
	Extra       map[string]any `json:"extra,omitempty"`
}

type UpdateCustomFieldRequest struct {
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Type        *string         `json:"type,omitempty"`
	Order       *int64          `json:"order,omitempty"`
	Extra       *map[string]any `json:"extra,omitempty"`
}

type CustomFieldValues struct {
	Resource         int64          `json:"-"`
	AttributesValues map[string]any `json:"attributes_values"`
	Version          int            `json:"version"`
}

type UpdateCustomFieldValuesRequest struct {
	AttributesValues map[string]any `json:"attributes_values"`
	Version          int            `json:"version"`
}

type ExtraInfo struct {
	ID       int64  `json:"id"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
	FullName string `json:"full_name_display,omitempty"`
}

type Issue struct {
	ID                  int64      `json:"id"`
	Ref                 int        `json:"ref"`
	Project             int64      `json:"project"`
	Subject             string     `json:"subject"`
	Description         string     `json:"description,omitempty"`
	Version             int        `json:"version"`
	Status              int64      `json:"status"`
	StatusExtraInfo     ExtraInfo  `json:"status_extra_info"`
	Priority            int64      `json:"priority"`
	PriorityExtraInfo   ExtraInfo  `json:"priority_extra_info"`
	Severity            int64      `json:"severity"`
	SeverityExtraInfo   ExtraInfo  `json:"severity_extra_info"`
	Type                int64      `json:"type"`
	TypeExtraInfo       ExtraInfo  `json:"type_extra_info"`
	AssignedTo          *int64     `json:"assigned_to"`
	AssignedToExtraInfo *ExtraInfo `json:"assigned_to_extra_info,omitempty"`
	IsClosed            bool       `json:"is_closed"`
	IsWatcher           bool       `json:"is_watcher"`
	IsVoter             bool       `json:"is_voter"`
	Tags                []Tag      `json:"tags,omitempty"`
	CreatedDate         string     `json:"created_date,omitempty"`
	ModifiedDate        string     `json:"modified_date,omitempty"`
}

type IssueStatus struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	IsClosed bool   `json:"is_closed"`
	Order    int    `json:"order"`
}

type NamedMetadata struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Order int    `json:"order"`
}

type CreateIssueRequest struct {
	Project     int64    `json:"project"`
	Subject     string   `json:"subject"`
	Description string   `json:"description,omitempty"`
	Status      *int64   `json:"status,omitempty"`
	Priority    *int64   `json:"priority,omitempty"`
	Severity    *int64   `json:"severity,omitempty"`
	Type        *int64   `json:"type,omitempty"`
	AssignedTo  *int64   `json:"assigned_to,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type UpdateIssueRequest struct {
	Version     int       `json:"version"`
	Subject     *string   `json:"subject,omitempty"`
	Description *string   `json:"description,omitempty"`
	Status      *int64    `json:"status,omitempty"`
	Priority    *int64    `json:"priority,omitempty"`
	Severity    *int64    `json:"severity,omitempty"`
	Type        *int64    `json:"type,omitempty"`
	AssignedTo  *int64    `json:"assigned_to,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
	Comment     *string   `json:"comment,omitempty"`
}

type UserStory struct {
	ID              int64            `json:"id"`
	Ref             int              `json:"ref"`
	Project         int64            `json:"project"`
	Subject         string           `json:"subject"`
	Description     string           `json:"description,omitempty"`
	Version         int              `json:"version"`
	Status          int64            `json:"status"`
	StatusExtraInfo ExtraInfo        `json:"status_extra_info"`
	Milestone       *int64           `json:"milestone"`
	MilestoneSlug   string           `json:"milestone_slug,omitempty"`
	MilestoneName   string           `json:"milestone_name,omitempty"`
	AssignedUsers   []int64          `json:"assigned_users,omitempty"`
	TotalPoints     *float64         `json:"total_points,omitempty"`
	Points          map[string]int64 `json:"points,omitempty"`
	IsClosed        bool             `json:"is_closed"`
	IsWatcher       bool             `json:"is_watcher"`
	IsVoter         bool             `json:"is_voter"`
	IsBlocked       bool             `json:"is_blocked"`
	BlockedNote     string           `json:"blocked_note,omitempty"`
	BacklogOrder    int64            `json:"backlog_order"`
	SprintOrder     int64            `json:"sprint_order"`
	KanbanOrder     int64            `json:"kanban_order"`
	Tags            []Tag            `json:"tags,omitempty"`
	CreatedDate     string           `json:"created_date,omitempty"`
	ModifiedDate    string           `json:"modified_date,omitempty"`
}

type UserStoryStatus struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	IsClosed bool   `json:"is_closed"`
	Order    int    `json:"order"`
}

type Milestone struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Slug            string  `json:"slug"`
	Project         int64   `json:"project"`
	Closed          bool    `json:"closed"`
	EstimatedStart  string  `json:"estimated_start,omitempty"`
	EstimatedFinish string  `json:"estimated_finish,omitempty"`
	CreatedDate     string  `json:"created_date,omitempty"`
	ModifiedDate    string  `json:"modified_date,omitempty"`
	Order           int     `json:"order"`
	Disponibility   float64 `json:"disponibility"`
}

type CreateMilestoneRequest struct {
	Project         int64  `json:"project"`
	Name            string `json:"name"`
	EstimatedStart  string `json:"estimated_start"`
	EstimatedFinish string `json:"estimated_finish"`
}

type UpdateMilestoneRequest struct {
	Name            *string `json:"name,omitempty"`
	EstimatedStart  *string `json:"estimated_start,omitempty"`
	EstimatedFinish *string `json:"estimated_finish,omitempty"`
	Closed          *bool   `json:"closed,omitempty"`
}

type CreateUserStoryRequest struct {
	Project       int64    `json:"project"`
	Subject       string   `json:"subject"`
	Description   string   `json:"description,omitempty"`
	Status        *int64   `json:"status,omitempty"`
	Milestone     *int64   `json:"milestone,omitempty"`
	AssignedUsers []int64  `json:"assigned_users,omitempty"`
	Tags          []string `json:"tags,omitempty"`
}

type UpdateUserStoryRequest struct {
	Version       int       `json:"version"`
	Subject       *string   `json:"subject,omitempty"`
	Description   *string   `json:"description,omitempty"`
	Status        *int64    `json:"status,omitempty"`
	Milestone     **int64   `json:"milestone,omitempty"`
	AssignedUsers *[]int64  `json:"assigned_users,omitempty"`
	Tags          *[]string `json:"tags,omitempty"`
	Comment       *string   `json:"comment,omitempty"`
}

type TaskStoryInfo struct {
	ID      int64  `json:"id"`
	Ref     int    `json:"ref"`
	Subject string `json:"subject"`
}

type Task struct {
	ID                  int64          `json:"id"`
	Ref                 int            `json:"ref"`
	Project             int64          `json:"project"`
	UserStory           *int64         `json:"user_story"`
	UserStoryExtraInfo  *TaskStoryInfo `json:"user_story_extra_info,omitempty"`
	Milestone           *int64         `json:"milestone"`
	MilestoneSlug       string         `json:"milestone_slug,omitempty"`
	Subject             string         `json:"subject"`
	Description         string         `json:"description,omitempty"`
	Version             int            `json:"version"`
	Status              int64          `json:"status"`
	StatusExtraInfo     ExtraInfo      `json:"status_extra_info"`
	AssignedTo          *int64         `json:"assigned_to"`
	AssignedToExtraInfo *ExtraInfo     `json:"assigned_to_extra_info,omitempty"`
	IsClosed            bool           `json:"is_closed"`
	IsWatcher           bool           `json:"is_watcher"`
	IsVoter             bool           `json:"is_voter"`
	IsBlocked           bool           `json:"is_blocked"`
	BlockedNote         string         `json:"blocked_note,omitempty"`
	CreatedDate         string         `json:"created_date,omitempty"`
	ModifiedDate        string         `json:"modified_date,omitempty"`
	FinishedDate        string         `json:"finished_date,omitempty"`
}

type TaskStatus struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	IsClosed bool   `json:"is_closed"`
	Order    int    `json:"order"`
}

type CreateTaskRequest struct {
	Project     int64  `json:"project"`
	Subject     string `json:"subject"`
	Description string `json:"description,omitempty"`
	UserStory   *int64 `json:"user_story,omitempty"`
	Status      *int64 `json:"status,omitempty"`
	AssignedTo  *int64 `json:"assigned_to,omitempty"`
}

type UpdateTaskRequest struct {
	Version     int     `json:"version"`
	Subject     *string `json:"subject,omitempty"`
	Description *string `json:"description,omitempty"`
	UserStory   **int64 `json:"user_story,omitempty"`
	Milestone   **int64 `json:"milestone,omitempty"`
	Status      *int64  `json:"status,omitempty"`
	AssignedTo  **int64 `json:"assigned_to,omitempty"`
	Comment     *string `json:"comment,omitempty"`
}

type WikiPage struct {
	ID           int64  `json:"id"`
	Project      int64  `json:"project"`
	Slug         string `json:"slug"`
	Content      string `json:"content"`
	HTML         string `json:"html,omitempty"`
	Owner        *int64 `json:"owner,omitempty"`
	LastModifier *int64 `json:"last_modifier,omitempty"`
	CreatedDate  string `json:"created_date,omitempty"`
	ModifiedDate string `json:"modified_date,omitempty"`
	Editions     int    `json:"editions"`
	Version      int    `json:"version"`
	IsWatcher    bool   `json:"is_watcher"`
}

type CreateWikiPageRequest struct {
	Project int64  `json:"project"`
	Slug    string `json:"slug"`
	Content string `json:"content"`
}

type UpdateWikiPageRequest struct {
	Version int     `json:"version"`
	Slug    *string `json:"slug,omitempty"`
	Content *string `json:"content,omitempty"`
}

type Epic struct {
	ID                  int64          `json:"id"`
	Ref                 int            `json:"ref"`
	Project             int64          `json:"project"`
	Subject             string         `json:"subject"`
	Description         string         `json:"description,omitempty"`
	Color               string         `json:"color,omitempty"`
	Version             int            `json:"version"`
	Status              int64          `json:"status"`
	StatusExtraInfo     ExtraInfo      `json:"status_extra_info"`
	AssignedTo          *int64         `json:"assigned_to"`
	AssignedToExtraInfo *ExtraInfo     `json:"assigned_to_extra_info,omitempty"`
	ClientRequirement   bool           `json:"client_requirement"`
	TeamRequirement     bool           `json:"team_requirement"`
	IsClosed            bool           `json:"is_closed"`
	IsBlocked           bool           `json:"is_blocked"`
	BlockedNote         string         `json:"blocked_note,omitempty"`
	IsWatcher           bool           `json:"is_watcher"`
	IsVoter             bool           `json:"is_voter"`
	UserStoriesCounts   map[string]any `json:"user_stories_counts,omitempty"`
	CreatedDate         string         `json:"created_date,omitempty"`
	ModifiedDate        string         `json:"modified_date,omitempty"`
}

type EpicStatus struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug,omitempty"`
	IsClosed bool   `json:"is_closed"`
	Order    int    `json:"order"`
}

type CreateEpicRequest struct {
	Project     int64  `json:"project"`
	Subject     string `json:"subject"`
	Description string `json:"description,omitempty"`
	Status      *int64 `json:"status,omitempty"`
	AssignedTo  *int64 `json:"assigned_to,omitempty"`
	Color       string `json:"color,omitempty"`
}

type UpdateEpicRequest struct {
	Version     int     `json:"version"`
	Subject     *string `json:"subject,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *int64  `json:"status,omitempty"`
	AssignedTo  **int64 `json:"assigned_to,omitempty"`
	Color       *string `json:"color,omitempty"`
}

type EpicRelatedUserStory struct {
	Epic      int64 `json:"epic"`
	UserStory int64 `json:"user_story"`
	Order     int64 `json:"order"`
}

type CreateEpicRelatedUserStoryRequest struct {
	Epic      int64 `json:"epic"`
	UserStory int64 `json:"user_story"`
}

type SearchItem struct {
	ID            int64    `json:"id"`
	Ref           int      `json:"ref,omitempty"`
	Slug          string   `json:"slug,omitempty"`
	Subject       string   `json:"subject,omitempty"`
	Status        *int64   `json:"status,omitempty"`
	AssignedTo    *int64   `json:"assigned_to,omitempty"`
	TotalPoints   *float64 `json:"total_points,omitempty"`
	MilestoneName string   `json:"milestone_name,omitempty"`
	MilestoneSlug string   `json:"milestone_slug,omitempty"`
}

type SearchResponse struct {
	Epics       []SearchItem `json:"epics,omitempty"`
	UserStories []SearchItem `json:"userstories,omitempty"`
	Tasks       []SearchItem `json:"tasks,omitempty"`
	Issues      []SearchItem `json:"issues,omitempty"`
	WikiPages   []SearchItem `json:"wikipages,omitempty"`
	Count       int          `json:"count"`
}

type Attachment struct {
	ID               int64  `json:"id"`
	Project          int64  `json:"project"`
	Owner            *int64 `json:"owner,omitempty"`
	ObjectID         int64  `json:"object_id"`
	Name             string `json:"name"`
	Size             int64  `json:"size"`
	URL              string `json:"url"`
	PreviewURL       string `json:"preview_url,omitempty"`
	ThumbnailCardURL string `json:"thumbnail_card_url,omitempty"`
	Description      string `json:"description,omitempty"`
	IsDeprecated     bool   `json:"is_deprecated"`
	FromComment      bool   `json:"from_comment"`
	SHA1             string `json:"sha1,omitempty"`
	CreatedDate      string `json:"created_date,omitempty"`
	ModifiedDate     string `json:"modified_date,omitempty"`
}

type UpdateAttachmentRequest struct {
	Description  *string `json:"description,omitempty"`
	IsDeprecated *bool   `json:"is_deprecated,omitempty"`
}

type HistoryUser struct {
	ID       *int64 `json:"pk"`
	Username string `json:"username,omitempty"`
	Name     string `json:"name,omitempty"`
}

type HistoryEntry struct {
	ID                string         `json:"id"`
	User              HistoryUser    `json:"user"`
	CreatedAt         string         `json:"created_at"`
	Type              int            `json:"type"`
	Diff              map[string]any `json:"diff,omitempty"`
	ValuesDiff        map[string]any `json:"values_diff,omitempty"`
	Comment           string         `json:"comment,omitempty"`
	DeleteCommentDate string         `json:"delete_comment_date,omitempty"`
	EditCommentDate   string         `json:"edit_comment_date,omitempty"`
	IsHidden          bool           `json:"is_hidden"`
	IsSnapshot        bool           `json:"is_snapshot"`
}
