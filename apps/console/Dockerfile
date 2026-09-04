FROM node:20-alpine
WORKDIR /app
COPY sdk/typescript ./sdk
COPY apps/console ./console
# SDK first: the console resolves @tide/sdk through its package exports.
WORKDIR /app/sdk
RUN if [ -d node_modules/.bin ]; then echo "sdk: using vendored node_modules"; else npm ci --no-audit --no-fund; fi \
 && npm run build
# Console: install (or reuse vendored), then repair the @tide/sdk symlink —
# COPY preserves it as a host-pointing link, so relink to the in-image SDK.
# file: installs are fully local (no registry).
WORKDIR /app/console
RUN if [ -d node_modules/.bin ]; then echo "console: using vendored node_modules"; else npm ci --no-audit --no-fund; fi \
 && npm install --no-save --no-audit --no-fund file:../sdk
# Never run as root (A02): node:20-alpine ships a `node` user.
RUN chown -R node:node /app
USER node
EXPOSE 5173
CMD ["npm", "run", "dev"]
