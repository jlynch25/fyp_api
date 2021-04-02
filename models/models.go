package model

import "go.mongodb.org/mongo-driver/bson/primitive"

type Event struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"`
	AuthorID string             `bson:"author_id"`
	Title    string             `bson:"title"`
	Content  string             `bson:"content"`
}

type User struct {
	ID            primitive.ObjectID   `bson:"_id,omitempty"`
	Email         string               `bson:"email"`
	Password      string               `bson:"password"`
	Name          string               `bson:"name"`
	PhoneNumber   string               `bson:"phoneNumber"`
	WalletAddress string               `bson:"walletAddress"`
	Friends       []primitive.ObjectID `bson:"user,omitempty"`
	// Events   []modelEvent       `bson:"events"`
}
