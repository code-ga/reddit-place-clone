# Use an official Go runtime as the build stage base image
FROM golang:1.17-alpine AS build

# Set the working directory inside the container
WORKDIR /app

# Copy the Go application source code into the container
COPY . .

# Build the Go application
RUN go build -o app

# Use a minimal base image for the final deployment stage
FROM alpine:3.14

# Set the working directory inside the container
WORKDIR /app

# Copy the built binary from the build stage
COPY --from=build /app/app .

# Expose the port the application will listen on
EXPOSE 8080

# Command to run the application
CMD ["./app"]
