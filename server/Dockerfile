FROM golang as build
WORKDIR /build
ENV CGO_ENABLED=0
COPY go.* .
RUN go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build go build -o place .


FROM alpine
WORKDIR /app

COPY --from=build /build/place .

EXPOSE 4000

CMD [ "/app/place", "--width", "1280", "--height", "720", "--save-location", "/place/place.png" ]
