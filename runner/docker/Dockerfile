FROM ubuntu:24.04 AS runner-base

ARG RUNNER_VERSION=2.338.1
ARG RUNNER_SHA256_LINUX_X64=08e9ccb3604d2462c24b5e33604dc30810296ca15cc65f23cffd4317eb144c37
ENV DEBIAN_FRONTEND=noninteractive

# Docker CLI and Compose talk to the Colima daemon through its socket.
RUN apt-get update
RUN apt-get install -y --no-install-recommends \
    bash build-essential ca-certificates curl git gnupg jq gosu \
    libicu74 libssl3t64 libkrb5-3 liblttng-ust1t64 libnuma1 \
    openssh-client python3 python3-venv python3-pip tar unzip zip \
    && install -m 0755 -d /etc/apt/keyrings \
    && curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
       | gpg --dearmor -o /etc/apt/keyrings/docker.gpg \
    && chmod a+r /etc/apt/keyrings/docker.gpg \
    && . /etc/os-release \
    && echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" \
       > /etc/apt/sources.list.d/docker.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
       docker-ce-cli docker-buildx-plugin docker-compose-plugin \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd -g 1001 runner && useradd -m -u 1001 -g runner -s /bin/bash runner \
    && mkdir -p /opt/actions-runner /runner \
    && curl -fL --retry 3 \
       "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/actions-runner-linux-x64-${RUNNER_VERSION}.tar.gz" \
       -o /tmp/runner.tar.gz \
    && printf '%s  %s\n' "$RUNNER_SHA256_LINUX_X64" /tmp/runner.tar.gz | sha256sum -c - \
    && tar -xzf /tmp/runner.tar.gz -C /opt/actions-runner \
    && rm /tmp/runner.tar.gz \
    && chown -R runner:runner /opt/actions-runner /runner

COPY entrypoint.sh /usr/local/bin/runner-entrypoint
RUN chmod +x /usr/local/bin/runner-entrypoint
WORKDIR /runner
ENTRYPOINT ["/usr/local/bin/runner-entrypoint"]
CMD ["run"]

# BookBuilder requires pinned tools beyond the minimal, reusable runner base.
# The PHP 8.2 packages come from the signed ondrej/php Ubuntu PPA; Noble defaults
# to PHP 8.3. Do not change the generic runner's dependency set.
FROM runner-base AS bookbuilder

ARG JUST_VERSION=1.58.0
ARG JUST_SHA256=4a5cc2f53e6f0f8c59092a6cc38291eb729d46a7dd95d3ae582008881b84931d
ARG TYPST_VERSION=0.15.1
ARG TYPST_SHA256=a6d077d0a95eed5a2eba715b2dae06be954f624ccbf85758a03f389ded33118c

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       software-properties-common ffmpeg fontconfig fonts-dejavu-core \
       fonts-liberation fonts-noto-core openjdk-21-jre-headless xz-utils \
    && add-apt-repository --yes ppa:ondrej/php \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
       php8.2-cli php8.2-curl php8.2-mbstring php8.2-zip \
    && rm -rf /var/lib/apt/lists/*

# Official GitHub release archives: versions and SHA-256 digests are pinned.
RUN curl -fsSL --retry 3 \
       "https://github.com/casey/just/releases/download/${JUST_VERSION}/just-${JUST_VERSION}-x86_64-unknown-linux-musl.tar.gz" \
       -o /tmp/just.tar.gz \
    && printf '%s  %s\\n' "$JUST_SHA256" /tmp/just.tar.gz | sha256sum -c - \
    && mkdir -p /tmp/just-unpack \
    && tar -xzf /tmp/just.tar.gz -C /tmp/just-unpack \
    && install -m 0755 "$(find /tmp/just-unpack -type f -name just -print -quit)" /usr/local/bin/just \
    && rm -rf /tmp/just.tar.gz /tmp/just-unpack

RUN curl -fsSL --retry 3 \
       "https://github.com/typst/typst/releases/download/v${TYPST_VERSION}/typst-x86_64-unknown-linux-musl.tar.xz" \
       -o /tmp/typst.tar.xz \
    && printf '%s  %s\\n' "$TYPST_SHA256" /tmp/typst.tar.xz | sha256sum -c - \
    && mkdir -p /tmp/typst-unpack \
    && tar -xJf /tmp/typst.tar.xz -C /tmp/typst-unpack \
    && install -m 0755 "$(find /tmp/typst-unpack -type f -name typst -print -quit)" /usr/local/bin/typst \
    && rm -rf /tmp/typst.tar.xz /tmp/typst-unpack

# Fail the image build if BookBuilder's pinned host requirements are not met.
RUN command -v just && command -v ffmpeg && command -v fc-list \
    && test "$(php -r 'echo PHP_MAJOR_VERSION.".".PHP_MINOR_VERSION;')" = "8.2" \
    && php -r 'foreach (["curl","zip","mbstring"] as $extension) { if (!extension_loaded($extension)) exit(1); }' \
    && java -version 2>&1 | grep -Eq 'version "21\\.|openjdk 21\\.' \
    && typst --version | grep -Eq '^typst 0[.]15[.]1([[:space:]]|$)' \
    && just --version
