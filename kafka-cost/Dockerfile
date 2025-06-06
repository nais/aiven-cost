FROM --platform=$BUILDPLATFORM rust:1 as builder

WORKDIR /build
ARG TARGETPLATFORM
RUN \
    set -eux ; \
    if [ "$TARGETPLATFORM" = "linux/amd64" ]; then \
        apt-get --yes update && apt-get --yes install cmake musl-tools ; \
        rustup target add x86_64-unknown-linux-musl ; \
    elif [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
        apt-get --yes update && apt-get --yes install cmake musl-tools gcc-aarch64-linux-gnu ; \
        rustup target add aarch64-unknown-linux-musl ; \
    fi

COPY . .

RUN \
    set -eux ; \
    if [ "$TARGETPLATFORM" = "linux/amd64" ]; then \
        export TARGET=x86_64-unknown-linux-musl ; \
    elif [ "$TARGETPLATFORM" = "linux/arm64" ]; then \
        export TARGET=aarch64-unknown-linux-musl ; \
        export CARGO_TARGET_AARCH64_UNKNOWN_LINUX_MUSL_LINKER=aarch64-linux-gnu-gcc ; \
        export CC_aarch64_unknown_linux_musl=aarch64-linux-gnu-gcc ; \
        export CXX_aarch64_unknown_linux_musl=aarch64-linux-gnu-g++ ; \
    fi ; \
    cargo test --release --target ${TARGET} -- --test-threads 1 ; \
    cargo build --release --target ${TARGET} && mkdir -p target/final/release/ && mv target/${TARGET}/release/kafka-cost target/final/release/kafka-cost ; \
    file target/final/release/kafka-cost


FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /build/target/final/release/kafka-cost /app/kafka-cost
EXPOSE 8999
ENV RUST_LOG="kafka-cost=info,kube=warn"
ENTRYPOINT ["/app/kafka-cost"]
