package main

import (
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	Id       int
	Username string
	Email    string
	Password string
	Chats    []int
}

func (u *User) ValidatePassword(pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(pw)) == nil
}

type UserJSON struct {
	Id       int        `json:"id"`
	Username string     `json:"username"`
	Email    string     `json:"email"`
	Chats    []ChatJSON `json:"chats"`
	Token    string     `json:"token"`
}

type Chat struct {
	Id       int
	Password string
	Messages []MessageJSON
	Users    []AuthorJSON
}

func (c *Chat) ValidatePassword(pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(c.Password), []byte(pw)) == nil
}

func (c *Chat) ToJSON() ChatJSON {
	return ChatJSON{
		Id:       c.Id,
		Messages: c.Messages,
		Users:    c.Users,
	}
}

type ChatJSON struct {
	Id       int           `json:"id"`
	Messages []MessageJSON `json:"messages"`
	Users    []AuthorJSON  `json:"users"`
}

type MessageJSON struct {
	ChatId int        `json:"chatId"`
	Text   string     `json:"text"`
	Author AuthorJSON `json:"author"`
}

type AuthorJSON struct {
	Id       int    `json:"id"`
	Username string `json:"username"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type CreateChatRequest struct {
	Password string `json:"password"`
}

type JoinChatRequest struct {
	Id       int    `json:"id"`
	Password string `json:"password"`
}

type ContextKey string
