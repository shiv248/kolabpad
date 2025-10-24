// Package protocol defines the WebSocket message protocol between client and server.
// This matches the Rustpad protocol exactly for wire compatibility.
package protocol

import (
	"encoding/json"

	"github.com/shiv248/kolabpad/pkg/ot"
)

// UserInfo represents a connected user's display information.
type UserInfo struct {
	Name string `json:"name"` // Display name
	Hue  uint32 `json:"hue"`  // Color hue (0-359)
}

// CursorData represents a user's cursor positions and selections.
type CursorData struct {
	Cursors    []uint32    `json:"cursors"`    // Cursor positions (Unicode codepoint offsets)
	Selections [][2]uint32 `json:"selections"` // Selection ranges [start, end]
}

// UserOperation represents an operation with the user ID who created it.
type UserOperation struct {
	ID        uint64           `json:"id"`        // User ID
	Operation *ot.OperationSeq `json:"operation"` // The OT operation
}

// ClientMsg represents messages sent from client to server.
// Only one field should be set per message (tagged union pattern).
type ClientMsg struct {
	Edit        *EditMsg    `json:"Edit,omitempty"`
	SetLanguage *string     `json:"SetLanguage,omitempty"`
	ClientInfo  *UserInfo   `json:"ClientInfo,omitempty"`
	CursorData  *CursorData `json:"CursorData,omitempty"`
}

// EditMsg represents a text edit operation from the client.
type EditMsg struct {
	Revision  int              `json:"revision"`  // Client's current revision
	Operation *ot.OperationSeq `json:"operation"` // The edit operation
}

// ServerMsg represents messages sent from server to client.
// Only one field should be set per message (tagged union pattern).
type ServerMsg struct {
	Identity   *uint64        `json:"Identity,omitempty"`
	History    *HistoryMsg    `json:"History,omitempty"`
	Language   *LanguageMsg   `json:"Language,omitempty"`
	UserInfo   *UserInfoMsg   `json:"UserInfo,omitempty"`
	UserCursor *UserCursorMsg `json:"UserCursor,omitempty"`
	OTP        *OTPMsg        `json:"OTP,omitempty"`
}

// HistoryMsg sends a batch of operations to the client.
type HistoryMsg struct {
	Start      int             `json:"start"`      // Starting revision number
	Operations []UserOperation `json:"operations"` // Operations from start to current
}

// UserInfoMsg broadcasts user connection/disconnection events.
type UserInfoMsg struct {
	ID   uint64    `json:"id"`             // User ID
	Info *UserInfo `json:"info,omitempty"` // User info, or nil if disconnected
}

// UserCursorMsg broadcasts cursor position updates.
type UserCursorMsg struct {
	ID   uint64     `json:"id"`   // User ID
	Data CursorData `json:"data"` // Cursor positions
}

// LanguageMsg broadcasts language changes to all clients.
type LanguageMsg struct {
	Language string `json:"language"`  // New language
	UserID   uint64 `json:"user_id"`   // User who made the change
	UserName string `json:"user_name"` // User's display name
}

// OTPMsg broadcasts OTP changes to authenticated clients.
type OTPMsg struct {
	OTP      *string `json:"otp"`       // OTP token, or nil if disabled
	UserID   uint64  `json:"user_id"`   // User who made the change
	UserName string  `json:"user_name"` // User's display name
}

// MarshalJSON implements custom JSON marshaling for ServerMsg.
// We need to ensure only one field is present in the JSON output.
func (m *ServerMsg) MarshalJSON() ([]byte, error) {
	// Create a map with only the non-nil field
	result := make(map[string]interface{})

	if m.Identity != nil {
		result["Identity"] = *m.Identity
	} else if m.History != nil {
		result["History"] = m.History
	} else if m.Language != nil {
		result["Language"] = m.Language
	} else if m.UserInfo != nil {
		result["UserInfo"] = m.UserInfo
	} else if m.UserCursor != nil {
		result["UserCursor"] = m.UserCursor
	} else if m.OTP != nil {
		result["OTP"] = m.OTP
	}

	return json.Marshal(result)
}

// UnmarshalJSON implements custom JSON unmarshaling for ClientMsg.
func (m *ClientMsg) UnmarshalJSON(data []byte) error {
	// First unmarshal into a generic map to see which field is present
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if editData, ok := raw["Edit"]; ok {
		var edit EditMsg
		if err := json.Unmarshal(editData, &edit); err != nil {
			return err
		}
		m.Edit = &edit
	}

	if langData, ok := raw["SetLanguage"]; ok {
		var lang string
		if err := json.Unmarshal(langData, &lang); err != nil {
			return err
		}
		m.SetLanguage = &lang
	}

	if infoData, ok := raw["ClientInfo"]; ok {
		var info UserInfo
		if err := json.Unmarshal(infoData, &info); err != nil {
			return err
		}
		m.ClientInfo = &info
	}

	if cursorData, ok := raw["CursorData"]; ok {
		var cursor CursorData
		if err := json.Unmarshal(cursorData, &cursor); err != nil {
			return err
		}
		m.CursorData = &cursor
	}

	return nil
}

// Helper constructors for server messages

// NewIdentityMsg creates an Identity server message.
func NewIdentityMsg(id uint64) *ServerMsg {
	return &ServerMsg{Identity: &id}
}

// NewHistoryMsg creates a History server message.
func NewHistoryMsg(start int, ops []UserOperation) *ServerMsg {
	return &ServerMsg{History: &HistoryMsg{Start: start, Operations: ops}}
}

// NewLanguageMsg creates a Language server message.
func NewLanguageMsg(lang string, userID uint64, userName string) *ServerMsg {
	return &ServerMsg{Language: &LanguageMsg{Language: lang, UserID: userID, UserName: userName}}
}

// NewUserInfoMsg creates a UserInfo server message.
func NewUserInfoMsg(id uint64, info *UserInfo) *ServerMsg {
	return &ServerMsg{UserInfo: &UserInfoMsg{ID: id, Info: info}}
}

// NewUserCursorMsg creates a UserCursor server message.
func NewUserCursorMsg(id uint64, data CursorData) *ServerMsg {
	return &ServerMsg{UserCursor: &UserCursorMsg{ID: id, Data: data}}
}

// NewOTPMsg creates an OTP server message.
func NewOTPMsg(otp *string, userID uint64, userName string) *ServerMsg {
	return &ServerMsg{OTP: &OTPMsg{OTP: otp, UserID: userID, UserName: userName}}
}
