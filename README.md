# Temporalize

Temporalize is a music trivia game where players guess details about songs.
Each card has a binary QR-Code on the back, making them indistinguishable to the human eye.
On the front, they have song details (Title, Artist, Year, Genre).
Players scan the cards with the web app, listen to the songs, and guess the year or other details.

## Prerequisites

*   **Go**: 1.22+
*   **Node.js**: 18+
*   **Python**: 3.x
*   **Task**: [go-task/task](https://taskfile.dev/) (Build tool)
*   **OpenSSL**: Required for generating self-signed certificates for the web app.

## Setup

1.  **Install Dependencies:**
    ```bash
    npm install
    ```

2.  **Environment Variables:**
    Create a `.env` file in the root directory with your Spotify API credentials. You can get these from the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard/).
    ```bash
    SPOTIFY_CLIENT_ID=your_client_id
    SPOTIFY_CLIENT_SECRET=your_client_secret
    ```

## Usage

This project uses a `Taskfile` to manage scripts.

### 1. Collect Songs
Collects top songs by popularity from Spotify for a given year range.

```bash
# Default (1970-2025)
task collect

# Custom Range
task collect START=2020 END=2024

# Custom Output
task collect OUTPUT=my_list.json
```

### 2. Generate Assets
Fetches metadata, thumbnails, and links for other platforms (Apple Music, Amazon Music, YouTube Music), then generates the card images and QR codes.

```bash
# Default (reads spotify_links.json, outputs to assets/generated)
task generate

# Custom Input/Output
task generate INPUT=my_list.json OUTPUT=assets/my_cards
```

### 3. Run Web App
Starts the QR code scanning web application.

```bash
# Default (Port 8000)
task web

# Custom Port
task web PORT=8080
```

**Note on SSL/HTTPS:**
The web app requires HTTPS to access the camera on mobile devices. The start script (`web/serve_https.py`) will automatically generate a self-signed SSL certificate (`server.pem` and `key.pem`) using `openssl` if they don't exist.
*   **Browser Warning:** When you first visit the site, your browser will warn you that the connection is not private. This is expected for a self-signed certificate. You must click "Advanced" -> "Proceed" (or "Accept Risk") to continue.
*   **Mobile Testing:** To test on your phone, ensure your phone and computer are on the same Wi-Fi network and visit `https://<YOUR_COMPUTER_IP>:<PORT>`.

## Architecture

*   **`cmd/collect`**: Go script to search Spotify for popular tracks.
*   **`cmd/generate`**: Go script to fetch cross-platform links (via Odesli), validate them, and generate card assets.
*   **`web/`**: TypeScript/HTML web application for scanning cards.
*   **`assets/`**: Stores generated images, QR codes, and thumbnails.

## QR Code Format
The QR codes use a custom binary encoding to minimize size.
Structure: `[AmazonAlbumID+Explicit (7 bytes), AmazonSongID (7 bytes), AppleAlbumID (Uvarint), AppleSongID (Varint Delta), SpotifyID (17 bytes), YouTubeID (9 bytes)]`
IDs are compressed using custom BaseN encoding (Base36/Base62/Base64) and packed into a binary format. The explicit flag is stored in the most significant bit of the first byte.
