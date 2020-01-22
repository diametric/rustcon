package webrcon

// Response contains root response format
type Response struct {
	Message    string `json:"Message"`
	Identifier int    `json:"Identifier"`
	Type       string `json:"Type"`
	Stacktrace string `json:"Stacktrace"`
}

// ChatMessage contains the webrcon chat message format for Type: Chat
type ChatMessage struct {
	Channel  int    `json:"Channel"`
	Message  string `json:"Message"`
	UserID   string `json:"UserId"`
	Username string `json:"Username"`
	Color    string `json:"Color"`
	Time     int    `json:"Time"`
}

// Command contains a standard WebRcon command structure
type Command struct {
	Identifier int    `json:"Identifier"`
	Message    string `json:"Message"`
	Name       string `json:"Name"`
}
