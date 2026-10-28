FROM node:22-bookworm-slim@sha256:de170b39439eb0e5a3444ad7d60b015614a381e10fecae2e30ec9e735122f38d

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl git \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 10001 --create-home --home-dir /home/agent agent

COPY --chmod=755 project-x /usr/local/bin/project-x

USER agent
ENV HOME=/home/agent
ENV PATH=/home/agent/.local/bin:$PATH

RUN mkdir -p /home/agent/.cursor /home/agent/.gemini/antigravity-cli /home/agent/.claude \
 && echo '{"mcpServers":{"project-x":{"command":"/usr/local/bin/project-x","args":["mcp"]}}}' > /home/agent/.cursor/mcp.json \
 && echo '{"mcpServers":{"project-x":{"command":"/usr/local/bin/project-x","args":["mcp"]}}}' > /home/agent/.gemini/antigravity-cli/mcp.json \
 && echo '{"mcpServers":{"project-x":{"command":"/usr/local/bin/project-x","args":["mcp"]}}}' > /home/agent/.claude/claude_desktop_config.json

RUN curl -fsS https://cursor.com/install | bash \
 && curl -fsSL https://antigravity.google/cli/install.sh | bash

RUN agent --version && agy --version && project-x --version || true
