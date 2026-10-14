FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        build-essential \
        dbus-x11 \
        libgtk-3-dev \
        libsecret-1-dev \
        libwebkit2gtk-4.1-dev \
        pkg-config \
        xvfb \
    && rm -rf /var/lib/apt/lists/*
