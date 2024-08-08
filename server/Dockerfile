
FROM rust:1.79.0-alpine 

COPY . ./build

RUN apk add --no-cache clang lld musl-dev git

RUN cargo install --path ./build

RUN rm -rf ./build

ENTRYPOINT [ "rplace_backend" ]

CMD ["--width", "2560", "--height", "1440", "--save-location", "/place/place.png", "--save_all_images","true"]
