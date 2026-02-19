# Use a Python base image
FROM python:3.10-slim

# Install Node.js, npm, and openssl
RUN apt-get update && apt-get install -y nodejs npm openssl

# Set the working directory
WORKDIR /app

# Copy package files and install dependencies
COPY package.json package-lock.json ./
RUN npm install

# Copy web app files
COPY web/ ./web/

# Compile TypeScript
RUN npx tsc web/app.ts --target es2020

# Expose the port the app runs on
EXPOSE 8000

# Run the web server
CMD ["python3", "web/serve_https.py", "8000"]
