FROM node:20-alpine
WORKDIR /app
COPY package.json ./
# Fresh clones / CI (network available): npm install.
# Hosts whose builder lacks registry DNS: `npm install` was already run in
# the working tree, so COPY brings node_modules along — use it as-is.
COPY . .
RUN if [ -d node_modules/.bin ]; then echo "console: using vendored node_modules"; else npm install --no-audit --no-fund; fi
EXPOSE 5173
CMD ["npm", "run", "dev"]
