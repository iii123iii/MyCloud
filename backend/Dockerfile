FROM ubuntu:24.04 AS builder

ARG MYCLOUD_VERSION

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    build-essential \
    cmake \
    git \
    libssl-dev \
    zlib1g-dev \
    libjsoncpp-dev \
    uuid-dev \
    libc-ares-dev \
    libmariadb-dev \
    libhiredis-dev \
    pkg-config \
    clang-18 \
    libc++-18-dev \
    libc++abi-18-dev \
    && rm -rf /var/lib/apt/lists/*

# Use Clang 18 for C++26
ENV CC=clang-18
ENV CXX=clang++-18

# Build Drogon from source
RUN git clone --depth=1 --branch v1.9.9 https://github.com/drogonframework/drogon.git /tmp/drogon \
    && cd /tmp/drogon \
    && git submodule update --init \
    && mkdir build && cd build \
    && cmake .. -DCMAKE_BUILD_TYPE=Release \
       -DBUILD_MYSQL=ON \
       -DBUILD_REDIS=ON \
       -DBUILD_TESTING=OFF \
    && make -j$(nproc) \
    && make install \
    && rm -rf /tmp/drogon

# Install jwt-cpp via cmake so picojson submodule is included
RUN git clone --depth=1 https://github.com/Thalhammer/jwt-cpp.git /tmp/jwt-cpp \
    && cd /tmp/jwt-cpp \
    && git submodule update --init \
    && cmake -B build \
       -DJWT_BUILD_EXAMPLES=OFF \
       -DJWT_BUILD_TESTS=OFF \
       -DJWT_DISABLE_PICOJSON=OFF \
    && cmake --install build \
    && rm -rf /tmp/jwt-cpp

WORKDIR /app
COPY . .

RUN rm -rf build && mkdir -p build && cd build \
    && cmake .. -DCMAKE_BUILD_TYPE=Release -DMYCLOUD_VERSION_OVERRIDE="${MYCLOUD_VERSION}" \
    && make -j$(nproc)

# ─── Runtime stage ───────────────────────────────────────────────
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    git \
    docker.io \
    docker-compose-v2 \
    mariadb-client \
    libssl3 \
    libmariadb3 \
    libhiredis1.1.0 \
    libjsoncpp25 \
    libc-ares2 \
    libuuid1 \
    zlib1g \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/build/mycloud_backend /usr/local/bin/mycloud_backend

# Pre-create the upload temp directory inside the storage volume mountpoint
RUN mkdir -p /data/files/tmp

EXPOSE 8080

CMD ["mycloud_backend"]
