FROM golang:alpine as golang-compiler
WORKDIR /build
ENV CGO_ENABLED=0
COPY go.* .
RUN go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build go build -o place .


FROM golang:alpine as golang-remover
WORKDIR /app
COPY --from=golang-compiler /build/place ./

FROM alpine
WORKDIR /app

COPY --from=golang-remover /app/place ./

EXPOSE 80

CMD [ "/app/place --save /place/place.png" ]
