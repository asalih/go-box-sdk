package schemas

// UserBase is the minimal user representation (id + type).
type UserBase struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// UserMini extends UserBase with the user's name and login.
type UserMini struct {
	UserBase
	Name  string `json:"name,omitempty"`
	Login string `json:"login,omitempty"`
}
