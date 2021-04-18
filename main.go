package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	model "github.com/jlynch25/fyp_api/models"
	pb "github.com/jlynch25/fyp_api/proto"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	db       *mongo.Client
	eventdb  *mongo.Collection
	userdb   *mongo.Collection
	mongoctx context.Context
)

// ServiceServer struct
type ServiceServer struct {
}

func main() {

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Unable to listen on port :50051: %v", err)
	}

	defer listener.Close()

	// cert
	cert, err := tls.LoadX509KeyPair("./cert/public.crt", "./cert/private.key")
	if err != nil {
		log.Fatalf("failed to load key pair: %s", err)
	}

	// gRPC
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(validateToken),
		grpc.Creds(credentials.NewServerTLSFromCert(&cert)),
	}
	grpcServer := grpc.NewServer(opts...)
	server := &ServiceServer{}
	pb.RegisterServiceServer(grpcServer, server)
	// reflection.Register(s)

	defer grpcServer.Stop()

	// MongoDB
	mongodbURL := fmt.Sprintf("mongodb://%s:%s@%s:%d/%s", "mongodb4221u", "mongodb4221u", "danu7.it.nuigalway.ie", 8717, "mongodb4221")
	mongoctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := mongo.Connect(mongoctx, options.Client().ApplyURI(mongodbURL))
	if err != nil {
		log.Fatal(err)
	}
	err = db.Ping(mongoctx, nil)
	if err != nil {
		log.Fatalf("Could not connect to MongoDB: %v\n", err)
	} else {
		fmt.Println("Connected to MongoDB")
	}
	eventdb = db.Database("mongodb4221").Collection("event")
	userdb = db.Database("mongodb4221").Collection("user")

	defer db.Disconnect(mongoctx)

	// Start the server in a goroutine
	go func() {
		if e := grpcServer.Serve(listener); e != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()
	fmt.Println("Server succesfully started on port :50051")
	defer fmt.Println("\nServer Stopped serving")

	// Stop the server using a SHUTDOWN HOOK
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt)
	<-c
}

func validateToken(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "missing metadata")
	}

	if !valid(md["authorization"]) {
		return nil, status.Errorf(codes.Unauthenticated, "invalid token")
	}

	return handler(ctx, req)
}

func valid(authorization []string) bool {
	if len(authorization) < 1 {
		return false
	}

	return true //FIXME - for now
	// token := strings.TrimPrefix(authorization[0], "Bearer ")

	// // If you have more than one client then you will have to update this line.
	// return token == "client-x-id"
}

func getUserIDStream(tokenString string) (string, error) {
	type MyCustomClaims struct {
		jwt.StandardClaims
	}

	token, err := jwt.ParseWithClaims(tokenString, &MyCustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte("pi_is_exactly_3"), nil
	})

	if err != nil {
		// return internal gRPC error to be handled later
		return "", status.Errorf(
			codes.Internal,
			fmt.Sprintf("Internal error: %v", err),
		)
	}

	userID := ""

	if claims, ok := token.Claims.(*MyCustomClaims); ok && token.Valid {
		userID = claims.Id
	} else {
		fmt.Println(ok)
	}
	return userID, nil
}

func getUserID(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)

	if !ok {
		return "", status.Errorf(codes.InvalidArgument, "missing metadata")
	}

	if len(md["authorization"]) < 1 {
		return "", status.Errorf(codes.Unauthenticated, "invalid token")
	}

	tokenString := strings.TrimPrefix(md["authorization"][0], "Bearer ")

	type MyCustomClaims struct {
		jwt.StandardClaims
	}

	token, err := jwt.ParseWithClaims(tokenString, &MyCustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte("pi_is_exactly_3"), nil
	})

	if err != nil {
		// return internal gRPC error to be handled later
		return "", status.Errorf(
			codes.Internal,
			fmt.Sprintf("Internal error: %v", err),
		)
	}

	userID := ""

	if claims, ok := token.Claims.(*MyCustomClaims); ok && token.Valid {
		userID = claims.Id
	} else {
		fmt.Println(ok)
	}

	return userID, nil
}

// EVENTS

// CreateEvent function
func (s *ServiceServer) CreateEvent(ctx context.Context, req *pb.CreateEventReq) (*pb.CreateEventRes, error) {
	// Essentially doing req.Event to access the struct with a nil check
	event := req.GetEvent()

	autherID, err := getUserID(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Internal error: %v", err))
	}

	// Now we have to convert this into a models.Event type to convert into BSON
	data := model.Event{
		// ID:    Empty, so it gets omitted and MongoDB generates a unique Object ID upon insertion.
		AuthorID: autherID,
		Title:    event.GetTitle(),
		Content:  event.GetContent(),
	}

	// Insert the data into the database, result contains the newly generated Object ID for the new document
	result, err := eventdb.InsertOne(mongoctx, data)
	// check for potential errors
	if err != nil {
		// return internal gRPC error to be handled later
		return nil, status.Errorf(
			codes.Internal,
			fmt.Sprintf("Internal error: %v", err),
		)
	}
	// add the id to event, first cast the "generic type" (go doesn't have real generics yet) to an Object ID.
	oid := result.InsertedID.(primitive.ObjectID)
	// Convert the object id to it's string counterpart
	event.Id = oid.Hex()
	// return the event in a CreateEventRes type
	return &pb.CreateEventRes{Event: event}, nil
}

// ReadEvent function
func (s *ServiceServer) ReadEvent(ctx context.Context, req *pb.ReadEventReq) (*pb.ReadEventRes, error) {
	// convert string id (from proto) to mongoDB ObjectId
	oid, err := primitive.ObjectIDFromHex(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("Could not convert to ObjectId: %v", err))
	}
	result := eventdb.FindOne(ctx, bson.M{"_id": oid})
	// Create an empty models.Event to write our decode result to
	data := model.Event{}
	// decode and write to data
	if err := result.Decode(&data); err != nil {
		return nil, status.Errorf(codes.NotFound, fmt.Sprintf("Could not find event with Object Id %s: %v", req.GetId(), err))
	}
	// Cast to ReadEventRes type
	response := &pb.ReadEventRes{
		Event: &pb.Event{
			Id:       oid.Hex(),
			AuthorId: data.AuthorID,
			Title:    data.Title,
			Content:  data.Content,
		},
	}
	return response, nil
}

// DeleteEvent function
func (s *ServiceServer) DeleteEvent(ctx context.Context, req *pb.DeleteEventReq) (*pb.DeleteEventRes, error) {
	// Get the ID (string) from the request message and convert it to an Object ID
	oid, err := primitive.ObjectIDFromHex(req.GetId())
	// Check for errors
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("Could not convert to ObjectId: %v", err))
	}
	// DeleteOne returns DeleteResult which is a struct containing the amount of deleted docs (in this case only 1 always)
	// So we return a boolean instead
	_, err = eventdb.DeleteOne(ctx, bson.M{"_id": oid})
	// Check for errors
	if err != nil {
		return nil, status.Errorf(codes.NotFound, fmt.Sprintf("Could not find/delete event with id %s: %v", req.GetId(), err))
	}
	// Return response with success: true if no error is thrown (and thus document is removed)
	return &pb.DeleteEventRes{
		Success: true,
	}, nil
}

// UpdateEvent function
func (s *ServiceServer) UpdateEvent(ctx context.Context, req *pb.UpdateEventReq) (*pb.UpdateEventRes, error) {
	// Get the event data from the request
	event := req.GetEvent()

	userID, err := getUserID(ctx)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("get user ID error: %v", err))
	}

	if userID != event.GetAuthorId() {
		return nil, status.Errorf(codes.Unauthenticated, fmt.Sprintf("Not authorized: %v", err))
	}

	// Convert the Id string to a MongoDB ObjectId
	oid, err := primitive.ObjectIDFromHex(event.GetId())
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			fmt.Sprintf("Could not convert the supplied event id to a MongoDB ObjectId: %v", err),
		)
	}

	// Convert the data to be updated into an unordered Bson document
	update := bson.M{
		"title":   event.GetTitle(),
		"content": event.GetContent(),
	}

	// Convert the oid into an unordered bson document to search by id
	filter := bson.M{"_id": oid}

	// Result is the BSON encoded result
	// To return the updated document instead of original we have to add options.
	result := eventdb.FindOneAndUpdate(ctx, filter, bson.M{"$set": update}, options.FindOneAndUpdate().SetReturnDocument(1))

	// Decode result and write it to 'decoded'
	decoded := model.Event{}
	err = result.Decode(&decoded)
	if err != nil {
		return nil, status.Errorf(
			codes.NotFound,
			fmt.Sprintf("Could not find event with supplied ID: %v", err),
		)
	}
	return &pb.UpdateEventRes{
		Event: &pb.Event{
			Id:       decoded.ID.Hex(),
			AuthorId: decoded.AuthorID,
			Title:    decoded.Title,
			Content:  decoded.Content,
		},
	}, nil
}

// ListEvents function
func (s *ServiceServer) ListEvents(req *pb.ListEventsReq, stream pb.Service_ListEventsServer) error {
	// Initiate a models.Event type to write decoded data to
	data := &model.Event{}
	// collection.Find returns a cursor for our (empty) query
	cursor, err := eventdb.Find(context.Background(), bson.M{})
	if err != nil {
		return status.Errorf(codes.Internal, fmt.Sprintf("Unknown internal error: %v", err))
	}
	// An expression with defer will be called at the end of the function
	defer cursor.Close(context.Background())
	// cursor.Next() returns a boolean, if false there are no more items and loop will break
	for cursor.Next(context.Background()) {
		// Decode the data at the current pointer and write it to data
		err := cursor.Decode(data)
		// check error
		if err != nil {
			return status.Errorf(codes.Unavailable, fmt.Sprintf("Could not decode data: %v", err))
		}
		// If no error is found send event over stream
		stream.Send(&pb.ListEventsRes{
			Event: &pb.Event{
				Id:       data.ID.Hex(),
				AuthorId: data.AuthorID,
				Content:  data.Content,
				Title:    data.Title,
			},
		})
	}
	// Check if the cursor has any errors
	if err := cursor.Err(); err != nil {
		return status.Errorf(codes.Internal, fmt.Sprintf("Unkown cursor error: %v", err))
	}
	return nil
}

// USERS

// CreateUser function
func (s *ServiceServer) CreateUser(ctx context.Context, req *pb.CreateUserReq) (*pb.CreateUserRes, error) {
	// Essentially doing req.User to access the struct with a nil check
	user := req.GetUser()

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.GetPassword()), bcrypt.DefaultCost)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, fmt.Sprintf("bcrypt GenerateFromPassword error for user %s: %v", user.GetEmail(), err))
	}

	userExists := userdb.FindOne(ctx, bson.M{"email": user.GetEmail()})
	if err := userExists.Decode(&model.User{}); err == nil {
		return nil, status.Errorf(codes.AlreadyExists, fmt.Sprintf("Email taken"))
	}

	// Now we have to convert this into a models.User type to convert into BSON
	data := model.User{
		// ID:    Empty, so it gets omitted and MongoDB generates a unique Object ID upon insertion.
		Name:     user.GetName(),
		Email:    user.GetEmail(),
		Password: string(hashedPassword),
	}

	// Insert the data into the database, result contains the newly generated Object ID for the new document
	result, err := userdb.InsertOne(mongoctx, data)
	// check for potential errors
	if err != nil {
		// return internal gRPC error to be handled later
		return nil, status.Errorf(
			codes.Internal,
			fmt.Sprintf("Internal error: %v", err),
		)
	}
	// add the id to user, first cast the "generic type" (go doesn't have real generics yet) to an Object ID.
	oid := result.InsertedID.(primitive.ObjectID)
	// Convert the object id to it's string counterpart
	user.Id = oid.Hex()

	// JWT
	// Create the Claims
	type MyCustomClaims struct {
		jwt.StandardClaims
	}

	claims := MyCustomClaims{
		jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Hour * 72).Unix(),
			Id:        oid.Hex(),
		},
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// Sign and get the complete encoded token as a string
	mySigningKey := []byte("pi_is_exactly_3")
	tokenString, err := token.SignedString(mySigningKey)

	if err != nil {
		return nil, status.Errorf(codes.NotFound, fmt.Sprintf("jwt error for user %s: %v", user.GetEmail(), err))
	}

	// Cast to CreateUserRes type
	response := &pb.CreateUserRes{
		User: &pb.User{
			Id: user.Id,
			// Name:  user.GetName(),
			Email: user.GetEmail(),
		},
		AccessToken: tokenString,
	}

	// return the user in a CreateUserRes type
	return response, nil
}

// LoginUser function
func (s *ServiceServer) LoginUser(ctx context.Context, req *pb.LoginUserReq) (*pb.LoginUserRes, error) {

	fmt.Println("LOGIN")
	fmt.Println(req.GetEmail())
	fmt.Println(req.GetPassword())
	result := userdb.FindOne(ctx, bson.M{"email": req.GetEmail()})
	// Create an empty models.User to write our decode result to
	data := model.User{}
	// decode and write to data
	if err := result.Decode(&data); err != nil {
		return nil, status.Errorf(codes.NotFound, fmt.Sprintf("Could not find user with email %s: %v", req.GetEmail(), err))
	}

	// Comparing the password with the hash
	err := bcrypt.CompareHashAndPassword([]byte(data.Password), []byte(req.GetPassword()))

	if err != nil {
		return nil, status.Errorf(codes.NotFound, fmt.Sprintf("password doesn't match for user %s: %v", req.GetEmail(), err))
	}

	// JWT
	// Create the Claims
	claims := &jwt.StandardClaims{
		ExpiresAt: time.Now().Add(time.Hour * 72).Unix(),
		Id:        data.ID.Hex(),
	}
	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// Sign and get the complete encoded token as a string
	mySigningKey := []byte("pi_is_exactly_3")
	tokenString, err := token.SignedString(mySigningKey)

	if err != nil {
		return nil, status.Errorf(codes.NotFound, fmt.Sprintf("jwt error for user %s: %v", req.GetEmail(), err))
	}

	var wallets []*pb.Wallet = []*pb.Wallet{}

	for i := range data.Wallets {
		wallets = append(wallets, &pb.Wallet{
			Title:   data.Wallets[i].Title,
			Address: data.Wallets[i].Address,
		})
	}

	// Cast to LoginUserRes type
	response := &pb.LoginUserRes{
		User: &pb.User{
			Id:          data.ID.Hex(),
			Name:        data.Name,
			Email:       data.Email,
			PhoneNumber: data.PhoneNumber,
			Wallets:     wallets,
		},
		AccessToken: tokenString,
	}
	return response, nil
}

// ReadUser function
func (s *ServiceServer) ReadUser(ctx context.Context, req *pb.ReadUserReq) (*pb.ReadUserRes, error) {
	// convert string id (from proto) to mongoDB ObjectId
	oid, err := primitive.ObjectIDFromHex(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("Could not convert to ObjectId: %v", err))
	}
	result := userdb.FindOne(ctx, bson.M{"_id": oid})
	// Create an empty models.User to write our decode result to
	data := model.User{}
	// decode and write to data
	if err := result.Decode(&data); err != nil {
		return nil, status.Errorf(codes.NotFound, fmt.Sprintf("Could not find user with Object Id %s: %v", req.GetId(), err))
	}

	var wallets []*pb.Wallet = []*pb.Wallet{}

	for i := range data.Wallets {
		wallets = append(wallets, &pb.Wallet{
			Title:   data.Wallets[i].Title,
			Address: data.Wallets[i].Address,
		})
	}

	// Cast to ReadUserRes type
	response := &pb.ReadUserRes{
		User: &pb.User{
			Id:          oid.Hex(),
			Name:        data.Name,
			Email:       data.Email,
			PhoneNumber: data.PhoneNumber,
			Wallets:     wallets,
		},
	}
	return response, nil
}

// DeleteUser function
func (s *ServiceServer) DeleteUser(ctx context.Context, req *pb.DeleteUserReq) (*pb.DeleteUserRes, error) {
	// Get the ID (string) from the request message and convert it to an Object ID
	userID, err := getUserID(ctx)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("get user ID error: %v", err))
	}

	oid, err := primitive.ObjectIDFromHex(userID)
	// Check for errors
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("Could not convert to ObjectId: %v", err))
	}
	// DeleteOne returns DeleteResult which is a struct containing the amount of deleted docs (in this case only 1 always)
	// So we return a boolean instead
	_, err = userdb.DeleteOne(ctx, bson.M{"_id": oid})
	// Check for errors
	if err != nil {
		return nil, status.Errorf(codes.NotFound, fmt.Sprintf("Could not find/delete user with id %s: %v", userID, err))
	}
	// Return response with success: true if no error is thrown (and thus document is removed)
	return &pb.DeleteUserRes{
		Success: true,
	}, nil
}

// UpdateUser function
func (s *ServiceServer) UpdateUser(ctx context.Context, req *pb.UpdateUserReq) (*pb.UpdateUserRes, error) {
	// Get the user data from the request
	user := req.GetUser()

	userID, err := getUserID(ctx)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("get user ID error: %v", err))
	}

	// Convert the Id string to a MongoDB ObjectId
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			fmt.Sprintf("Could not convert the supplied user id to a MongoDB ObjectId: %v", err),
		)
	}

	update := bson.M{}

	if !(user.GetName() != "" && user.GetEmail() != "" && user.GetPassword() != "" && user.GetPhoneNumber() != "") {

		if user.GetPassword() != "" {

			hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.GetPassword()), bcrypt.DefaultCost)
			if err != nil {
				return nil, status.Errorf(codes.NotFound, fmt.Sprintf("bcrypt GenerateFromPassword error for user %s: %v", user.GetEmail(), err))
			}
			// Convert the data to be updated into an unordered Bson document
			update["password"] = string(hashedPassword)
		}
		if user.GetName() != "" {
			update["name"] = user.GetName()
		}
		if user.GetEmail() != "" {
			update["email"] = user.GetEmail()
		}
		if user.GetPhoneNumber() != "" {
			update["phoneNumber"] = user.GetPhoneNumber()
		}

	} else {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("No data to change"))
	}

	// Convert the oid into an unordered bson document to search by id
	filter := bson.M{"_id": oid}

	// Result is the BSON encoded result
	// To return the updated document instead of original we have to add options.
	result := userdb.FindOneAndUpdate(ctx, filter, bson.M{"$set": update}, options.FindOneAndUpdate().SetReturnDocument(1))

	// Decode result and write it to 'decoded'
	decoded := model.User{}
	err = result.Decode(&decoded)
	if err != nil {
		return nil, status.Errorf(
			codes.NotFound,
			fmt.Sprintf("Could not find user with supplied ID: %v", err),
		)
	}

	var wallets []*pb.Wallet = []*pb.Wallet{}

	for i := range decoded.Wallets {
		wallets = append(wallets, &pb.Wallet{
			Title:   decoded.Wallets[i].Title,
			Address: decoded.Wallets[i].Address,
		})
	}

	return &pb.UpdateUserRes{
		User: &pb.User{
			Id:          decoded.ID.Hex(),
			Name:        decoded.Name,
			Email:       decoded.Email,
			PhoneNumber: decoded.PhoneNumber,
			Wallets:     wallets,
		},
	}, nil
}

// ListUsers function
func (s *ServiceServer) ListUsers(req *pb.ListUsersReq, stream pb.Service_ListUsersServer) error {
	// Initiate a models.User type to write decoded data to
	data := &model.User{}
	// collection.Find returns a cursor for our (empty) query
	cursor, err := userdb.Find(context.Background(), bson.M{})
	if err != nil {
		return status.Errorf(codes.Internal, fmt.Sprintf("Unknown internal error: %v", err))
	}
	// An expression with defer will be called at the end of the function
	defer cursor.Close(context.Background())
	// cursor.Next() returns a boolean, if false there are no more items and loop will break
	for cursor.Next(context.Background()) {
		// Decode the data at the current pointer and write it to data
		err := cursor.Decode(data)
		// check error
		if err != nil {
			return status.Errorf(codes.Unavailable, fmt.Sprintf("Could not decode data: %v", err))
		}
		// If no error is found send user over stream
		stream.Send(&pb.ListUsersRes{
			User: &pb.User{
				Id:    data.ID.Hex(),
				Name:  data.Name,
				Email: data.Email,
			},
		})
	}
	// Check if the cursor has any errors
	if err := cursor.Err(); err != nil {
		return status.Errorf(codes.Internal, fmt.Sprintf("Unkown cursor error: %v", err))
	}
	return nil
}

func (s *ServiceServer) AddFriendUser(ctx context.Context, req *pb.UpdateFriendUserReq) (*pb.UpdateFriendUserRes, error) {

	friends := req.GetId()

	userID, err := getUserID(ctx)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("get user ID error: %v", err))
	}

	// Convert the Id string to a MongoDB ObjectId
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			fmt.Sprintf("Could not convert the supplied user id to a MongoDB ObjectId: %v", err),
		)
	}

	var update []primitive.ObjectID

	if friends != nil {

		for _, id := range friends {

			oid, err := primitive.ObjectIDFromHex(id)
			if err == nil {
				friend := userdb.FindOne(ctx, bson.M{"_id": oid})

				decoded := model.User{}
				err = friend.Decode(&decoded)
				if err == nil {
					update = append(update, oid)
				}
			}
		}

	} else {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("No data to change"))
	}

	filter := bson.M{"_id": oid}

	result := userdb.FindOneAndUpdate(ctx, filter, bson.M{"$addToSet": bson.M{"friends": bson.M{"$each": update}}}, options.FindOneAndUpdate().SetReturnDocument(1))

	decoded := model.User{}
	err = result.Decode(&decoded)
	if err != nil {
		return nil, status.Errorf(
			codes.NotFound,
			fmt.Sprintf("Could not find user with supplied ID: %v", err),
		)
	}
	// Return response with success: true if no error is thrown (and thus document is removed)
	return &pb.UpdateFriendUserRes{
		Success: true,
	}, nil
}

func (s *ServiceServer) RemoveFriendUser(ctx context.Context, req *pb.UpdateFriendUserReq) (*pb.UpdateFriendUserRes, error) {

	friends := req.GetId()

	userID, err := getUserID(ctx)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("get user ID error: %v", err))
	}

	// Convert the Id string to a MongoDB ObjectId
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			fmt.Sprintf("Could not convert the supplied user id to a MongoDB ObjectId: %v", err),
		)
	}

	var update []primitive.ObjectID

	if friends != nil {

		for _, id := range friends {
			oid, err := primitive.ObjectIDFromHex(id)
			if err == nil {
				update = append(update, oid)
			}
		}

	} else {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("No data to change"))
	}

	filter := bson.M{"_id": oid}

	result := userdb.FindOneAndUpdate(ctx, filter, bson.M{"$pullAll": bson.M{"friends": update}}, options.FindOneAndUpdate().SetReturnDocument(1))

	decoded := model.User{}
	err = result.Decode(&decoded)
	if err != nil {
		return nil, status.Errorf(
			codes.NotFound,
			fmt.Sprintf("Could not find user with supplied ID: %v", err),
		)
	}
	// Return response with success: true if no error is thrown (and thus document is removed)
	return &pb.UpdateFriendUserRes{
		Success: true,
	}, nil
}

func (s *ServiceServer) ListFriendsUser(req *pb.ListFriendsUserReq, stream pb.Service_ListFriendsUserServer) error {
	userID, err := getUserIDStream(req.GetAccessToken())
	if err != nil {
		return status.Errorf(codes.InvalidArgument, fmt.Sprintf("get user ID error: %v", err))
	}
	fmt.Println(userID)
	// Convert the Id string to a MongoDB ObjectId
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return status.Errorf(
			codes.InvalidArgument,
			fmt.Sprintf("Could not convert the supplied user id to a MongoDB ObjectId: %v", err),
		)
	}

	filter := bson.M{"_id": oid}

	result := userdb.FindOne(context.Background(), filter)

	data := model.User{}
	if err := result.Decode(&data); err != nil {
		return status.Errorf(codes.NotFound, fmt.Sprintf("Could not find user with Object Id %s: %v", userID, err))
	}
	fmt.Println(data)
	// var dataFriends []model.User = []model.User{}

	for _, friendID := range data.Friends {

		filter := bson.M{"_id": friendID}
		result := userdb.FindOne(context.Background(), filter)
		// data := model.User{}
		err := result.Decode(&data)
		fmt.Println(data)

		if err == nil {
			stream.Send(&pb.ListFriendsUserRes{
				User: &pb.User{
					Id:          oid.Hex(),
					Name:        data.Name,
					Email:       data.Email,
					PhoneNumber: data.PhoneNumber,
					// Wallet: data.Wallet,
				},
			})
		}
	}

	return nil
}

func (s *ServiceServer) ListXFriendsUser(ctx context.Context, req *pb.ListXFriendsUserReq) (*pb.ListXFriendsUserRes, error) {

	userID, err := getUserID(ctx)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("get user ID error: %v", err))
	}

	// Convert the Id string to a MongoDB ObjectId
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			fmt.Sprintf("Could not convert the supplied user id to a MongoDB ObjectId: %v", err),
		)
	}

	filter := bson.M{"_id": oid}

	result := userdb.FindOne(context.Background(), filter)

	data := model.User{}
	if err := result.Decode(&data); err != nil {
		return nil, status.Errorf(codes.NotFound, fmt.Sprintf("Could not find user with Object Id %s: %v", userID, err))
	}

	var friends []*pb.User = []*pb.User{}
	var wallets []*pb.Wallet = []*pb.Wallet{}

	limitsize, err := strconv.ParseInt(req.GetNumber(), 10, 32)

	if err == nil {

	}

	for i, friendID := range data.Friends {

		if i >= int(limitsize) {
			break
		}

		filter := bson.M{"_id": friendID}
		// options.FindOne().SetProjection(bson.M{"name": 0}
		result := userdb.FindOne(context.Background(), filter)

		err := result.Decode(&data)
		if err == nil {
			for i := range data.Wallets {
				wallets = append(wallets, &pb.Wallet{
					Title:   data.Wallets[i].Title,
					Address: data.Wallets[i].Address,
				})
			}

			friends = append(friends, &pb.User{
				Name:    data.Name,
				Wallets: wallets,
			})

			fmt.Println(data)
		}
	}

	// Cast to ReadUserRes type
	response := &pb.ListXFriendsUserRes{
		Friends: friends,
	}
	return response, nil
}

func (s *ServiceServer) AddWalletUser(ctx context.Context, req *pb.UpdateWalletUserReq) (*pb.UpdateWalletUserRes, error) {
	wallet := req.GetWallet()

	userID, err := getUserID(ctx)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("get user ID error: %v", err))
	}

	// Convert the Id string to a MongoDB ObjectId
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			fmt.Sprintf("Could not convert the supplied user id to a MongoDB ObjectId: %v", err),
		)
	}

	if wallet != nil {

		if wallet.Address != "" || wallet.Title != "" {

			filter := bson.M{"_id": oid}

			result := userdb.FindOneAndUpdate(ctx, filter, bson.M{"$addToSet": bson.M{"wallets": wallet}}, options.FindOneAndUpdate().SetReturnDocument(1))

			decoded := model.User{}
			err = result.Decode(&decoded)
			if err != nil {
				return nil, status.Errorf(
					codes.NotFound,
					fmt.Sprintf("Could not find user with supplied ID: %v", err),
				)
			}
			// Return response with success: true if no error is thrown (and thus document is removed)
			return &pb.UpdateWalletUserRes{
				Success: true,
			}, nil
		}
	}

	return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("No data to change"))
}

func (s *ServiceServer) RemoveWalletUser(ctx context.Context, req *pb.UpdateWalletUserReq) (*pb.UpdateWalletUserRes, error) {
	wallet := req.GetWallet()

	userID, err := getUserID(ctx)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("get user ID error: %v", err))
	}

	// Convert the Id string to a MongoDB ObjectId
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			fmt.Sprintf("Could not convert the supplied user id to a MongoDB ObjectId: %v", err),
		)
	}

	if wallet != nil {

		if wallet.Address != "" || wallet.Title != "" {

			filter := bson.M{"_id": oid}

			result := userdb.FindOneAndUpdate(ctx, filter, bson.M{"$pullAll": bson.M{"wallets": wallet}}, options.FindOneAndUpdate().SetReturnDocument(1))

			decoded := model.User{}
			err = result.Decode(&decoded)
			if err != nil {
				return nil, status.Errorf(
					codes.NotFound,
					fmt.Sprintf("Could not find user with supplied ID: %v", err),
				)
			}
			// Return response with success: true if no error is thrown (and thus document is removed)
			return &pb.UpdateWalletUserRes{
				Success: true,
			}, nil
		}
	}

	return nil, status.Errorf(codes.InvalidArgument, fmt.Sprintf("No data to change"))
}

func (srv *ServiceServer) Download(req *pb.DownloadReq, responseStream pb.Service_DownloadServer) error {
	fmt.Println("HERE download")
	bufferSize := 64 * 1024 //64KiB, tweak this as desired
	file, err := os.Open("tmp/blocks_gen/" + req.GetFileName())
	// fmt.Println(file)
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer file.Close()
	buff := make([]byte, bufferSize)
	for {
		bytesRead, err := file.Read(buff)
		if err != nil {
			if err != io.EOF {
				fmt.Println(err)
			}
			break
		}
		resp := &pb.Chunk{
			Content: buff[:bytesRead],
		}
		// fmt.Println(resp)
		err = responseStream.Send(resp)
		if err != nil {
			log.Println("error while sending chunk:", err)
			return err
		}
	}
	return nil
}
