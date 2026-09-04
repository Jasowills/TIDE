FROM node:20-alpine
WORKDIR /app
COPY package.json package-lock.json ./
# Fresh clones / CI (network available): npm ci from the committed lockfile.
# Hosts whose builder lacks registry DNS: `npm install` was already run in
# the working tree, so COPY brings node_modules along — use it as-is.
COPY . .
RUN if [ -d node_modules/.bin ]; then echo "console: using vendored node_modules"; else npm ci --no-audit --no-fund; fi
# Never run as root (A02): node:20-alpine ships a `node` user.
RUN chown -R node:node /app
USER node
EXPOSE 5173
CMD ["npm", "run", "dev"]
