package model

import "go.mongodb.org/mongo-driver/bson/primitive"

type Event struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"`
	AuthorID string             `bson:"author_id"`
	Title    string             `bson:"title"`
	Content  string             `bson:"content"`
}

type User struct {
	ID       primitive.ObjectID `bson:"_id,omitempty"`
	Email    string             `bson:"email"`
	Password string             `bson:"password"`
	Name     string             `bson:"name"`
	// Events   []modelEvent       `bson:"events"`
	// Friends  []modelUser        `bson:"friends"`
}
