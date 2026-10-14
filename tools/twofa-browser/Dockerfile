FROM mcr.microsoft.com/playwright:v1.56.1-noble
WORKDIR /opt/kitty-login-browser
COPY package.json package-lock.json ./
ENV PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
RUN npm ci --omit=dev --ignore-scripts --no-audit --no-fund && npm cache clean --force
COPY core.mjs session.mjs server.mjs ./
USER pwuser
ENV PORT=8080
CMD ["xvfb-run", "-a", "node", "server.mjs"]
