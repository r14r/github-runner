FROM ubuntu:24.04

ARG RUNNER_VERSION=2.338.1
ARG RUNNER_SHA256_LINUX_X64=08e9ccb3604d2462c24b5e33604dc30810296ca15cc65f23cffd4317eb144c37
ENV DEBIAN_FRONTEND=noninteractive

# Docker CLI and Compose talk to the Colima daemon through its socket.
RUN apt-get update && apt-get install -y --no-install-recommends \
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
