# ============================================
# Stage 1: Build OpenCV 4.12 headless
# ============================================
FROM golang:1.24-bookworm AS opencv-builder

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential cmake git wget unzip pkg-config \
    libavcodec-dev libavformat-dev libswscale-dev \
    libjpeg-dev libpng-dev libtiff-dev \
    libtbb-dev libeigen3-dev \
    libwebp-dev \
    && rm -rf /var/lib/apt/lists/*

# Download OpenCV 4.12 and contrib
ARG OPENCV_VERSION="4.12.0"
WORKDIR /tmp

RUN wget -q -O opencv.zip https://github.com/opencv/opencv/archive/${OPENCV_VERSION}.zip && \
    unzip -q opencv.zip && \
    wget -q -O opencv_contrib.zip https://github.com/opencv/opencv_contrib/archive/${OPENCV_VERSION}.zip && \
    unzip -q opencv_contrib.zip && \
    rm opencv.zip opencv_contrib.zip

# Build OpenCV (headless but with highgui for GoCV compatibility)
WORKDIR /tmp/opencv-${OPENCV_VERSION}/build

RUN cmake \
    -D CMAKE_BUILD_TYPE=RELEASE \
    -D CMAKE_INSTALL_PREFIX=/usr/local \
    -D OPENCV_EXTRA_MODULES_PATH=/tmp/opencv_contrib-${OPENCV_VERSION}/modules \
    -D OPENCV_ENABLE_NONFREE=ON \
    -D WITH_TBB=ON \
    -D WITH_FFMPEG=ON \
    -D WITH_WEBP=ON \
    -D WITH_V4L=OFF \
    -D WITH_GTK=OFF \
    -D WITH_QT=OFF \
    -D WITH_OPENGL=OFF \
    -D BUILD_EXAMPLES=OFF \
    -D BUILD_TESTS=OFF \
    -D BUILD_PERF_TESTS=OFF \
    -D BUILD_opencv_python2=OFF \
    -D BUILD_opencv_python3=OFF \
    -D BUILD_opencv_java=OFF \
    -D BUILD_opencv_apps=OFF \
    -D BUILD_DOCS=OFF \
    -D OPENCV_GENERATE_PKGCONFIG=ON \
    -D ENABLE_FAST_MATH=ON \
    -D WITH_EIGEN=ON \
    -D BUILD_opencv_highgui=ON \
    .. && \
    make -j$(nproc) && \
    make install && \
    ldconfig

# Verify installation
RUN echo "=== OpenCV Installation Check ===" && \
    ls -la /usr/local/lib/ | grep opencv | head -20

# ============================================
# Stage 2: Build Go application
# ============================================
FROM golang:1.24-bookworm AS go-builder

# Copy OpenCV from previous stage
COPY --from=opencv-builder /usr/local/include/opencv4/ /usr/local/include/opencv4/
COPY --from=opencv-builder /usr/local/lib/libopencv* /usr/local/lib/
COPY --from=opencv-builder /usr/local/lib/pkgconfig/ /usr/local/lib/pkgconfig/
COPY --from=opencv-builder /usr/local/lib/cmake/ /usr/local/lib/cmake/

# Install runtime dependencies for build
RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config \
    libtbb-dev \
    libavcodec-dev libavformat-dev libswscale-dev \
    libjpeg-dev libpng-dev \
    libwebp-dev \
    wget ca-certificates \
    ffmpeg file \
    && rm -rf /var/lib/apt/lists/*

# Set environment for CGO
ENV PKG_CONFIG_PATH=/usr/local/lib/pkgconfig
ENV LD_LIBRARY_PATH=/usr/local/lib
ENV CGO_ENABLED=1
ENV CGO_CPPFLAGS="-I/usr/local/include/opencv4"
ENV CGO_LDFLAGS="-L/usr/local/lib"

# Run ldconfig to update library cache
RUN ldconfig

WORKDIR /app

# Download Haar cascade for face detection
RUN wget -q https://raw.githubusercontent.com/opencv/opencv/4.12.0/data/haarcascades/haarcascade_frontalface_default.xml

# Copy Go modules first (for better caching)
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . ./



# Build binary with optimizations
RUN go build -ldflags="-s -w" -o mezon-bot .

# Verify binary
RUN echo "=== Binary dependencies ===" && \
    ldd mezon-bot && \
    echo "" && \
    file mezon-bot

# ============================================
# Stage 3: Runtime minimal container
# ============================================
FROM debian:bookworm-slim

# Install runtime libraries only
RUN apt-get update && apt-get install -y --no-install-recommends \
    libtbb12 \
    libavcodec59 \
    libavformat59 \
    libswscale6 \
    libjpeg62-turbo \
    libpng16-16 \
    libtiff6 \
    libwebpdemux2 \
    libwebpmux3 \
    libwebp7 \
    libgomp1 \
    ffmpeg \
    ca-certificates \
    procps \
    && rm -rf /var/lib/apt/lists/*

# Copy ALL OpenCV libraries
COPY --from=opencv-builder /usr/local/lib/libopencv*.so* /usr/local/lib/

# Update library cache
RUN ldconfig

WORKDIR /app

# Copy binary and Haar cascade
COPY --from=go-builder /app/mezon-bot ./
COPY --from=go-builder /app/haarcascade_frontalface_default.xml ./
COPY --from=go-builder /app/audio/* ./audio/
COPY --from=go-builder /app/config/* ./config/
# Test binary dependencies
RUN echo "=== Testing binary ===" && \
    ldd ./mezon-bot

# Create directories for captures/logs
RUN mkdir -p /app/image-captures /app/logs

# Set environment
ENV LD_LIBRARY_PATH=/usr/local/lib

# Non-root user for security
RUN useradd -m -u 1000 mezonbot && \
    chown -R mezonbot:mezonbot /app
USER mezonbot

# Healthcheck
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD pgrep -u mezonbot mezon-bot || exit 1

# Run bot
CMD ["./mezon-bot"]