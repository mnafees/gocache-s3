FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /gocache-s3 .

FROM gcr.io/distroless/static-debian12
COPY --from=build /gocache-s3 /gocache-s3
ENTRYPOINT ["/gocache-s3"]
