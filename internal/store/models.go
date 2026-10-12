package store

// User is an account that can sign in. The first user is the admin; admins
// manage other users and see every group.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Group is one pot of shared costs with a main currency.
type Group struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Currency    string `json:"currency"`
	CreatedBy   int64  `json:"created_by"`
	Archived    bool   `json:"archived"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`

	// Joined / derived, read-only.
	Members      []Member `json:"members"`
	ExpenseCount int      `json:"expense_count"`
	Total        int64    `json:"total"`         // spent (reimbursements excluded), group minor units
	MyMemberID   int64    `json:"my_member_id"`  // the viewer's member in this group, 0 if none
	MyBalance    int64    `json:"my_balance"`    // the viewer's balance, group minor units
	LastActivity string   `json:"last_activity"` // date of the latest expense
}

// Member takes part in a group; optionally a signed-in user.
type Member struct {
	ID        int64  `json:"id"`
	GroupID   int64  `json:"group_id"`
	UserID    *int64 `json:"user_id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// Expense is one payment by one member for some members.
type Expense struct {
	ID            int64   `json:"id"`
	GroupID       int64   `json:"group_id"`
	Title         string  `json:"title"`
	Amount        int64   `json:"amount"`
	Currency      string  `json:"currency"`
	FxRate        float64 `json:"fx_rate"`
	AmountGroup   int64   `json:"amount_group"`
	PaidBy        int64   `json:"paid_by"`
	Date          string  `json:"date"`
	Category      string  `json:"category"`
	Notes         string  `json:"notes"`
	SplitMode     string  `json:"split_mode"`
	Reimbursement bool    `json:"reimbursement"`
	CreatedBy     int64   `json:"created_by"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
	Shares        []Share `json:"shares"`
}

// Share is one participant's part of an expense.
type Share struct {
	MemberID    int64 `json:"member_id"`
	Share       int64 `json:"share"`
	Amount      int64 `json:"amount"`
	AmountGroup int64 `json:"amount_group"`
}

// Activity is one line of the group's feed.
type Activity struct {
	ID        int64  `json:"id"`
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	UserID    int64  `json:"user_id"`
	UserName  string `json:"user_name"`
	Kind      string `json:"kind"`
	ExpenseID int64  `json:"expense_id"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"created_at"`
}

// Settings are preferences stored as key/value pairs, shared by everyone.
type Settings struct {
	Title           string `json:"title"`
	DefaultCurrency string `json:"default_currency"`
	FxAuto          bool   `json:"fx_auto"` // look exchange rates up automatically
	SetupComplete   bool   `json:"setup_complete"`
}

// Categories an expense can have. Fixed list, like Spliit's: enough to see
// where the money went, not a taxonomy to maintain.
var Categories = []struct {
	ID, Label, Icon string
}{
	{"general", "General", "🧾"},
	{"groceries", "Groceries", "🛒"},
	{"restaurant", "Restaurants & cafés", "🍽️"},
	{"drinks", "Drinks & nightlife", "🍻"},
	{"transport", "Transport", "🚆"},
	{"fuel", "Fuel & parking", "⛽"},
	{"accommodation", "Accommodation", "🏨"},
	{"entertainment", "Entertainment", "🎟️"},
	{"shopping", "Shopping", "🛍️"},
	{"health", "Health", "💊"},
	{"utilities", "Rent & utilities", "🏠"},
	{"gifts", "Gifts", "🎁"},
	{"travel", "Flights & travel", "✈️"},
	{"other", "Other", "📦"},
}

// ValidCategory reports whether id is a known category.
func ValidCategory(id string) bool {
	for _, c := range Categories {
		if c.ID == id {
			return true
		}
	}
	return false
}
