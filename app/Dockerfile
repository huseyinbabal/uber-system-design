FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /location-service .

FROM gcr.io/distroless/static
COPY --from=build /location-service /location-service
ENTRYPOINT ["/location-service"]
