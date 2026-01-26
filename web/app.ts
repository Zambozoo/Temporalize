// State
let videoStream: MediaStream | null = null;
let scanning = false;

// DOM Elements
const startBtn = document.getElementById('start-btn')!;
const videoContainer = document.getElementById('video-container')!;
const video = document.getElementById('video') as HTMLVideoElement;
const canvas = document.getElementById('canvas') as HTMLCanvasElement;
const canvasCtx = canvas.getContext('2d')!;
const resultDiv = document.getElementById('result')!;
const resetBtn = document.getElementById('reset-btn')!;
const allowExplicitCheckbox = document.getElementById('allow-explicit') as HTMLInputElement;

// Icons
const ICONS = {
    spotify: '<img src="icons/spotify.png" class="icon" alt="Spotify">',
    apple: '<img src="icons/applemusic.png" class="icon" alt="Apple Music">',
    amazon: '<img src="icons/amazonmusic.png" class="icon" alt="Amazon Music">',
    youtube: '<img src="icons/youtubemusic.png" class="icon" alt="YouTube Music">'
};

// Alphabets for Decompression
const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";
const base36Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ";
const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";

function reset() {
    stopScanner();
    resultDiv.style.display = 'none';
    resultDiv.innerHTML = '';
    resetBtn.style.display = 'none';
    startScanner();
}

async function startScanner() {
    startBtn.style.display = 'none';
    videoContainer.style.display = 'block';
    
    try {
        videoStream = await navigator.mediaDevices.getUserMedia({ 
            video: { facingMode: "environment" } 
        });
        video.srcObject = videoStream;
        video.setAttribute("playsinline", "true"); // required to tell iOS safari we don't want fullscreen
        video.play();
        scanning = true;
        requestAnimationFrame(tick);
    } catch (err) {
        console.error("Error accessing camera:", err);
        alert("Error accessing camera. Please ensure you have given permission.");
        startBtn.style.display = 'block';
        videoContainer.style.display = 'none';
    }
}

function stopScanner() {
    scanning = false;
    if (videoStream) {
        videoStream.getTracks().forEach(track => track.stop());
        videoStream = null;
    }
}

function tick() {
    if (!scanning) return;

    if (video.readyState === video.HAVE_ENOUGH_DATA) {
        canvas.height = video.videoHeight;
        canvas.width = video.videoWidth;
        canvasCtx.drawImage(video, 0, 0, canvas.width, canvas.height);
        
        const imageData = canvasCtx.getImageData(0, 0, canvas.width, canvas.height);
        
        // Use jsQR global from script tag
        const code = (window as any).jsQR(imageData.data, imageData.width, imageData.height, {
            inversionAttempts: "dontInvert",
        });

        if (code) {
            // Found a QR code!
            console.log("Found QR code", code);
            
            // code.binaryData is Uint8ClampedArray (0-255)
            const bytes: number[] = Array.from(code.binaryData);
            
            try {
                const decoded = decompress(bytes);

                // Check explicit content permission
                if (decoded.explicit && !allowExplicitCheckbox.checked) {
                    stopScanner();
                    videoContainer.style.display = 'none';
                    
                    let html = `<div style="width: 100%; text-align: center; margin-bottom: 15px;">
                        <div style="color: #d32f2f; font-weight: bold; border: 2px solid #d32f2f; padding: 5px; display: inline-block; border-radius: 4px;">EXPLICIT CONTENT</div>
                        <p style="margin-top: 10px; color: #666;">Explicit content is not allowed.</p>
                    </div>`;
                    
                    resultDiv.innerHTML = html;
                    resultDiv.style.display = 'block';
                    resetBtn.style.display = 'block';
                    return;
                }

                const links = getAllLinks(decoded);
                
                if (links.length > 0) {
                    stopScanner();
                    videoContainer.style.display = 'none';
                    
                    // Populate the div with buttons for all found links
                    let html = '';
                    
                    // Explicit badge is ONLY shown if blocked (handled above), so we don't show it here if allowed.

                    const isSafariBrowser = isSafari();

                    html += '<div class="result-links">';
                    links.forEach(item => {
                        const icon = ICONS[item.platform as keyof typeof ICONS] || '';
                        const btnClass = `btn-${item.platform} platform-btn`;
                        const label = item.platform.charAt(0).toUpperCase() + item.platform.slice(1);
                        
                        if (isSafariBrowser) {
                            html += `
                                <a class="${btnClass}" href="${item.link}" title="Open in ${label}">
                                    ${icon}
                                </a>
                            `;
                        } else {
                            html += `
                                <button class="${btnClass}" onclick="startCountdown('${item.link}', this)" title="Open in ${label}">
                                    ${icon}
                                </button>
                            `;
                        }
                    });
                    html += '</div>';
                    
                    resultDiv.innerHTML = html;
                    resultDiv.style.display = 'block';
                    resetBtn.style.display = 'block';
                } else {
                    stopScanner();
                    videoContainer.style.display = 'none';
                    
                    let html = `<div style="width: 100%; text-align: center; margin-bottom: 15px;">
                        <div style="color: #666; font-weight: bold; border: 2px solid #666; padding: 5px; display: inline-block; border-radius: 4px;">NO LINKS FOUND</div>
                        <p style="margin-top: 10px; color: #666;">Could not find any valid links on this card.</p>
                    </div>`;
                    
                    resultDiv.innerHTML = html;
                    resultDiv.style.display = 'block';
                    resetBtn.style.display = 'block';
                }
            } catch (e) {
                console.error("Decompression failed", e);
                // Continue scanning if decompression fails
            }
            return;
        }
    }
    requestAnimationFrame(tick);
}

// --- Decompression Logic ---

interface DecodedData {
    explicit: boolean;
    amazonAlbum: string;
    amazonTrack: string;
    appleAlbum: string;
    appleTrack: string;
    spotify: string;
    youtube: string;
}

// BigInt polyfill-ish for BaseN decoding
function decodeBaseN(s: string, alphabet: string): bigint {
    let val = 0n;
    const base = BigInt(alphabet.length);
    
    for (let i = 0; i < s.length; i++) {
        const char = s[i];
        const idx = alphabet.indexOf(char);
        if (idx === -1) throw new Error(`Invalid char ${char}`);
        val = val * base + BigInt(idx);
    }
    return val;
}

function encodeBaseN(val: bigint, alphabet: string): string {
    if (val === 0n) return alphabet[0];
    let res = "";
    const base = BigInt(alphabet.length);
    while (val > 0n) {
        const mod = val % base;
        res = alphabet[Number(mod)] + res;
        val = val / base;
    }
    return res;
}

function padString(s: string, length: number, padChar: string): string {
    while (s.length < length) {
        s = padChar + s;
    }
    return s;
}

function bytesToBigInt(bytes: number[]): bigint {
    let val = 0n;
    for (const b of bytes) {
        val = (val << 8n) | BigInt(b);
    }
    return val;
}

function readUvarint(bytes: number[], offset: number): { val: bigint, n: number } {
    let x = 0n;
    let s = 0n;
    for (let i = 0; ; i++) {
        if (offset + i >= bytes.length) throw new Error("buffer overflow");
        const b = BigInt(bytes[offset + i]);
        if (b < 0x80n) {
            x |= b << s;
            return { val: x, n: i + 1 };
        }
        x |= (b & 0x7fn) << s;
        s += 7n;
    }
}

function readVarint(bytes: number[], offset: number): { val: bigint, n: number } {
    const { val: ux, n } = readUvarint(bytes, offset);
    let x = ux >> 1n;
    if ((ux & 1n) !== 0n) {
        x = ~x;
    }
    return { val: x, n };
}

function decompress(data: number[]): DecodedData {
    if (data.length < 7) throw new Error("short data");
    
    let idx = 0;
    
    // Amazon Album + Explicit (7 bytes)
    const amzAlbBytes = data.slice(idx, idx + 7);
    idx += 7;
    
    // Extract Explicit
    const explicit = (amzAlbBytes[0] & 0x80) !== 0;
    // Clear Explicit bit
    amzAlbBytes[0] &= 0x7F;
    
    let amazonAlbum = "";
    const amzAlbVal = bytesToBigInt(amzAlbBytes);
    if (amzAlbVal > 0n) {
        amazonAlbum = padString(encodeBaseN(amzAlbVal, base36Chars), 10, base36Chars[0]);
    }
    
    // Amazon Track (7 bytes)
    if (idx + 7 > data.length) throw new Error("short data amz trk");
    const amzTrkBytes = data.slice(idx, idx + 7);
    idx += 7;
    let amazonTrack = "";
    const amzTrkVal = bytesToBigInt(amzTrkBytes);
    if (amzTrkVal > 0n) {
        amazonTrack = padString(encodeBaseN(amzTrkVal, base36Chars), 10, base36Chars[0]);
    }
    
    // Apple Album (Uvarint)
    const { val: appAlbVal, n: n1 } = readUvarint(data, idx);
    idx += n1;
    let appleAlbum = "";
    if (appAlbVal > 0n) {
        appleAlbum = appAlbVal.toString();
    }
    
    // Apple Track (Varint Delta)
    const { val: delta, n: n2 } = readVarint(data, idx);
    idx += n2;
    let appleTrack = "";
    const appTrkVal = BigInt(appleAlbum || 0) + delta;
    if (appTrkVal > 0n) {
        appleTrack = appTrkVal.toString();
    }
    
    // Spotify (17 bytes)
    if (idx + 17 > data.length) throw new Error("short data spot");
    const spotBytes = data.slice(idx, idx + 17);
    idx += 17;
    let spotify = "";
    const spotVal = bytesToBigInt(spotBytes);
    if (spotVal > 0n) {
        spotify = padString(encodeBaseN(spotVal, base62Chars), 22, base62Chars[0]);
    }
    
    // YouTube (9 bytes)
    if (idx + 9 > data.length) throw new Error("short data yt");
    const ytBytes = data.slice(idx, idx + 9);
    idx += 9;
    let youtube = "";
    const ytVal = bytesToBigInt(ytBytes);
    if (ytVal > 0n) {
        youtube = padString(encodeBaseN(ytVal, base64Chars), 11, base64Chars[0]);
    }
    
    return { explicit, amazonAlbum, amazonTrack, appleAlbum, appleTrack, spotify, youtube };
}

interface PlatformLink {
    platform: string;
    link: string;
}

function getAllLinks(decoded: DecodedData): PlatformLink[] {
    const links: PlatformLink[] = [];
    
    if (decoded.spotify) {
        links.push({
            platform: 'spotify',
            link: `https://open.spotify.com/track/${decoded.spotify}?go=1`
        });
    }
    
    if (decoded.appleAlbum && decoded.appleTrack) {
        links.push({
            platform: 'apple',
            link: `https://music.apple.com/us/album/${decoded.appleAlbum}?i=${decoded.appleTrack}&autoplay=true`
        });
    }
    
    if (decoded.amazonAlbum && decoded.amazonTrack) {
        let link = `https://music.amazon.com/albums/${decoded.amazonAlbum}?do=play&trackAsin=${decoded.amazonTrack}`;
        links.push({
            platform: 'amazon',
            link: link
        });
    }
    
    if (decoded.youtube) {
        links.push({
            platform: 'youtube',
            link: `https://music.youtube.com/watch?v=${decoded.youtube}`
        });
    }
    
    return links;
}

function isSafari(): boolean {
    const ua = navigator.userAgent;
    return ua.includes("Safari");
}

// Expose functions to window
(window as any).startScanner = startScanner;
(window as any).reset = reset;
(window as any).startCountdown = startCountdown;

function startCountdown(url: string, btn: HTMLButtonElement) {
    let count = 3;
    const originalContent = btn.innerHTML;
    
    // Disable interaction
    btn.disabled = true;
    btn.innerHTML = `<span style="font-size: 20px;">${count}</span>`;
    
    const interval = setInterval(() => {
        count--;
        if (count > 0) {
             btn.innerHTML = `<span style="font-size: 20px;">${count}</span>`;
        } else {
            clearInterval(interval);
            window.open(url, '_blank');
            // Restore after a bit in case user returns
            setTimeout(() => {
                btn.innerHTML = originalContent;
                btn.disabled = false;
            }, 2000);
        }
    }, 1000);
}
