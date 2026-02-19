import http.server
import ssl
import os
import sys

# Ensure we are in the web directory
script_dir = os.path.dirname(os.path.abspath(__file__))
os.chdir(script_dir)

# Default port
port = 8000
if len(sys.argv) > 1:
    try:
        port = int(sys.argv[1])
    except ValueError:
        print(f"Invalid port number: {sys.argv[1]}")
        sys.exit(1)

dev_mode = os.environ.get('DEV_MODE', '').lower() == 'true'
tls_pem_path = os.environ.get('TLS_PEM_PATH')
tls_key_path = os.environ.get('TLS_KEY_PATH')

use_ssl = False
cert_file = None
key_file = None

if dev_mode:
    # Generate self-signed cert if it doesn't exist
    if not os.path.exists("cert.pem") or not os.path.exists("key.pem"):
        print("Generating self-signed certificate...")
        os.system("openssl req -new -x509 -keyout key.pem -out cert.pem -days 365 -nodes -subj '/CN=localhost'")
    cert_file = "cert.pem"
    key_file = "key.pem"
    use_ssl = True
elif tls_pem_path:
    cert_file = tls_pem_path
    key_file = tls_key_path
    use_ssl = True

server_address = ('0.0.0.0', port)
httpd = http.server.HTTPServer(server_address, http.server.SimpleHTTPRequestHandler)

if use_ssl:
    print(f"Starting HTTPS server on port {port}...")
    print(f"Access at: https://<YOUR_IP>:{port}")
    if dev_mode:
        print("Note: You will see a security warning. Click 'Advanced' -> 'Proceed' to continue.")
    
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(certfile=cert_file, keyfile=key_file)
    httpd.socket = context.wrap_socket(httpd.socket, server_side=True)
else:
    print(f"Starting HTTP server on port {port}...")
    print(f"Access at: http://<YOUR_IP>:{port}")

httpd.serve_forever()
