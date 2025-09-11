FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY main.go .

# Build the application
RUN go build -o sample-app main.go

# Use a smaller base image for the final image
FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/sample-app .

# Expose the port the app runs on
EXPOSE 8080

# Command to run the application
CMD ["./sample-app"]