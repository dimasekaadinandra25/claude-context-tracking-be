FROM mcr.microsoft.com/playwright:v1.48.0-jammy

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev --no-audit --no-fund
COPY alerts.mjs hooks.mjs report.mjs ./
RUN mkdir /state && chown pwuser /state

USER pwuser
CMD ["node", "report.mjs"]
