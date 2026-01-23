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
            
            const decoded = decodeIDs(bytes);

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

            const links = getAllLinks(decoded.ids);
            
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
            return;
        }
    }
    requestAnimationFrame(tick);
}

function decodeIDs(bytes: number[]): { ids: string[], explicit: boolean } {
    const ids: string[] = [];
    let currentBytes: number[] = [];
    let explicit = false;

    if (bytes.length > 0) {
        // First byte is explicit flag
        explicit = bytes[0] === 1;

        for (let i = 1; i < bytes.length; i++) {
            let b = bytes[i];
            
            // Our encoding logic: first byte of each string is +128.
            // So if b >= 128, it's a start of a new string.
            if (b >= 128) {
                // Push previous string if exists
                if (currentBytes.length > 0) {
                    ids.push(String.fromCharCode(...currentBytes));
                }
                // Start new string with the offset removed
                currentBytes = [b - 128];
            } else {
                currentBytes.push(b);
            }
        }
        // Push last string
        if (currentBytes.length > 0) {
            ids.push(String.fromCharCode(...currentBytes));
        }
    }
    
    return { ids, explicit };
}

interface PlatformLink {
    platform: string;
    link: string;
}

function getAllLinks(ids: string[]): PlatformLink[] {
    const links: PlatformLink[] = [];
    
    // We have a list of IDs. We need to identify them.
    // Patterns:
    // Spotify: 22 chars, alphanumeric (Base62)
    // YouTube: 11 chars, alphanumeric (Base64)
    // Apple: Numeric
    // Amazon: Starts with B0 (ASIN)
    
    const spotifyId = ids.find(id => id.length === 22 && /^[a-zA-Z0-9]+$/.test(id));
    const youtubeId = ids.find(id => id.length === 11 && /^[a-zA-Z0-9_-]+$/.test(id));
    
    // Apple and Amazon might be split.
    // Apple parts are numeric.
    const appleParts = ids.filter(id => /^\d+$/.test(id));
    
    // Amazon parts start with B0
    const amazonParts = ids.filter(id => /^B0[A-Z0-9]{8}$/.test(id));
    
    if (spotifyId) {
        links.push({
            platform: 'spotify',
            link: `https://open.spotify.com/track/${spotifyId}?go=1`
        });
    }
    
    if (appleParts.length >= 2) {
        links.push({
            platform: 'apple',
            link: `https://music.apple.com/us/album/${appleParts[0]}?i=${appleParts[1]}&autoplay=true`
        });
    }
    
    if (amazonParts.length == 2) {
        let link = `https://music.amazon.com/albums/${amazonParts[0]}?do=play&trackAsin=${amazonParts[1]}`;
        links.push({
            platform: 'amazon',
            link: link
        });
    }
    
    if (youtubeId) {
        links.push({
            platform: 'youtube',
            link: `https://music.youtube.com/watch?v=${youtubeId}`
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
