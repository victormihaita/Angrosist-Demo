# syntax=docker/dockerfile:1

# ---- build stage ----------------------------------------------------------
FROM node:22-alpine AS build
WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY . .
# VITE_* is baked at build time. The browser (on the host) calls the backend's
# published port, so this defaults to localhost:8080. Override per environment.
ARG VITE_API_URL=http://localhost:8080
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build

# ---- runtime stage (static SPA via nginx) ---------------------------------
FROM nginx:1.27-alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
