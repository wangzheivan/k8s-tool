ARG BASE_IMAGE=alpine:3.20
ARG NODE_IMAGE=public.ecr.aws/docker/library/node:22-alpine

FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS frontend-builder

WORKDIR /src/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS server-builder

WORKDIR /src

COPY go.mod ./
COPY cmd ./cmd

ARG TARGETOS=linux
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/k8s-tool-server ./cmd/server

FROM ${BASE_IMAGE}

ARG KUBECTL_VERSION=stable
ARG TARGETARCH

RUN apk add --no-cache \
    bash \
    bind-tools \
    ca-certificates \
    curl \
    iproute2 \
    iputils \
    jq \
    net-tools \
    nginx \
    procps \
    tini \
    util-linux

RUN set -eux; \
    case "${TARGETARCH:-amd64}" in \
      amd64) kubectl_arch="amd64" ;; \
      arm64) kubectl_arch="arm64" ;; \
      *) echo "Unsupported TARGETARCH: ${TARGETARCH}" >&2; exit 1 ;; \
    esac; \
    if [ "${KUBECTL_VERSION}" = "stable" ]; then \
      resolved_version="$(curl -fsSL https://dl.k8s.io/release/stable.txt)"; \
    else \
      resolved_version="${KUBECTL_VERSION}"; \
    fi; \
    curl -fsSLo /usr/local/bin/kubectl "https://dl.k8s.io/release/${resolved_version}/bin/linux/${kubectl_arch}/kubectl"; \
    chmod +x /usr/local/bin/kubectl; \
    kubectl version --client=true

COPY nginx.conf /etc/nginx/nginx.conf
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY --from=server-builder /out/k8s-tool-server /usr/local/bin/k8s-tool-server
COPY --from=frontend-builder /src/frontend/dist /usr/local/share/k8s-tool/frontend

RUN chmod +x /usr/local/bin/entrypoint.sh \
    && chmod +x /usr/local/bin/k8s-tool-server \
    && mkdir -p /run/nginx /usr/share/nginx/html

EXPOSE 80

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/entrypoint.sh"]
