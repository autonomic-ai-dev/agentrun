FROM node:22-bookworm-slim@sha256:de170b39439eb0e5a3444ad7d60b015614a381e10fecae2e30ec9e735122f38d

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl git \
 && rm -rf /var/lib/apt/lists/* \
 && curl -fsSLo /tmp/go.tar.gz https://go.dev/dl/go1.27.2.linux-arm64.tar.gz \
 && echo "94f3e30b8e374bc285e7dadc11e0865726b9bc6e85b841ccceaabc0214c6b7c8  /tmp/go.tar.gz" | sha256sum -c - \
 && tar -C /usr/local -xzf /tmp/go.tar.gz \
 && rm /tmp/go.tar.gz

RUN npm install -g @anthropic-ai/claude-code@2.1.299

ENV PATH=/usr/local/go/bin:$PATH
ENV HOME=/tmp/agent-home
ENV GOPATH=/tmp/go
ENV GOCACHE=/tmp/go-cache
