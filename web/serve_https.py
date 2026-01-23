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

# Generate self-signed cert if it doesn't exist
if not os.path.exists("server.pem") or not os.path.exists("key.pem"):
    print("Generating self-signed certificate...")
    os.system("openssl req -new -x509 -keyout key.pem -out cert.pem -days 365 -nodes -subj '/CN=localhost'")

print(f"Starting HTTPS server on port {port}...")
print(f"Access at: https://<YOUR_IP>:{port}")
print("Note: You will see a security warning. Click 'Advanced' -> 'Proceed' to continue.")

server_address = ('0.0.0.0', port)
httpd = http.server.HTTPServer(server_address, http.server.SimpleHTTPRequestHandler)

context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.load_cert_chain(certfile='cert.pem', keyfile='key.pem')

httpd.socket = context.wrap_socket(httpd.socket, server_side=True)
httpd.serve_forever()
