// Copyright 2026 The MathWorks, Inc.

// matlab-proxy frontend logic
(function() {
    "use strict";

    let config = {};
    let clientId = generateId();
    let authToken = "";
    let statusPollTimer = null;
    let shutdownCountdown = null;
    let lastStatus = {};
    let previousStatus = "down";
    let currentOverlay = ""; // tracks which overlay template is showing
    let wasEverActive = false; // true once this tab has been the active client

    function generateId() {
        const arr = new Uint8Array(16);
        crypto.getRandomValues(arr);
        return Array.from(arr, b => b.toString(16).padStart(2, "0")).join("");
    }

    // --- API calls ---

    function api(method, path, body) {
        const opts = {
            method: method,
            credentials: "same-origin",
            headers: {}
        };
        if (authToken) {
            opts.headers["mwi-auth-token"] = authToken;
        }
        if (body) {
            opts.headers["Content-Type"] = "application/json";
            opts.body = JSON.stringify(body);
        }
        const url = config.baseURL + path;
        return fetch(url, opts).then(function(r) {
            if (!r.ok) {
                console.error("API error:", method, path, r.status, r.statusText);
                return null;
            }
            return r.json();
        }).catch(function(err) {
            console.error("API fetch error:", method, path, err);
            return null;
        });
    }

    function pollStatus() {
        const params = new URLSearchParams({MWI_CLIENT_ID: clientId, IS_DESKTOP: "TRUE"});
        api("GET", "/get_status?" + params.toString()).then(data => {
            if (!data) return;
            lastStatus = data;
            updateUI(data);
        });
    }

    // --- UI updates ---

    function updateUI(data) {
        var matlab = data.matlab || {};
        var status = matlab.status || "down";
        var isActiveClient = data.isActiveClient;
        var container = document.getElementById("matlab-frame-container");
        var existingFrame = document.getElementById("matlab-frame");
        var trigger = document.getElementById("trigger");

        // Update trigger appearance
        trigger.className = "trigger trigger-" + status;

        // Handle concurrency — only the active client gets the iframe
        if (config.concurrencyEnabled && isActiveClient === false) {
            // This tab is NOT the active client
            if (existingFrame) {
                // Remove iframe — session moved to another tab/browser
                container.innerHTML = '<div id="matlab-placeholder"><img src="' + config.baseURL + '/static/icons/matlab-logo.svg" alt="MATLAB" class="placeholder-logo" onerror="this.style.display=\'none\'"></div>';
            }

            // Show appropriate dialog (don't override auth or shutdown overlays)
            if (currentOverlay !== "tmpl-auth" && currentOverlay !== "tmpl-shutdown-warning") {
                if (wasEverActive) {
                    showOverlay("tmpl-session-transferred");
                } else {
                    showOverlay("tmpl-concurrent-session");
                }
            }
            previousStatus = status;
            return;
        }

        // This tab IS the active client (or concurrency is disabled)
        if (isActiveClient === true) {
            wasEverActive = true;

            // If we just reclaimed the session, close the concurrent session dialog
            if (currentOverlay === "tmpl-concurrent-session" || currentOverlay === "tmpl-session-transferred") {
                document.getElementById("overlay").classList.add("hidden");
                currentOverlay = "";
            }
        }

        // Update iframe if MATLAB is up and we're active
        if (status === "up" && !existingFrame) {
            // Build the fully qualified base URL for the mre parameter
            var baseUrl = window.location.protocol + '//' + window.location.host + config.baseURL;
            var iframeSrc = config.baseURL + '/index-jsd-cr.html?mre=' + encodeURIComponent(baseUrl) + '&websocket=on';
            if (authToken) {
                iframeSrc += '&mwi-auth-token=' + encodeURIComponent(authToken);
            }
            container.innerHTML = '<iframe id="matlab-frame" src="' + iframeSrc + '" frameborder="0"></iframe>';
        } else if (status !== "up" && existingFrame) {
            container.innerHTML = '<div id="matlab-placeholder"><img src="' + config.baseURL + '/static/icons/matlab-logo.svg" alt="MATLAB" class="placeholder-logo" onerror="this.style.display=\'none\'"></div>';
        }

        // Auto-close the overlay when MATLAB transitions to "up"
        if (status === "up" && previousStatus !== "up") {
            if (currentOverlay === "tmpl-status-panel" || currentOverlay === "tmpl-licensing-panel") {
                document.getElementById("overlay").classList.add("hidden");
                currentOverlay = "";
            }
        }

        // Refresh the status panel content if it's currently showing
        if (currentOverlay === "tmpl-status-panel") {
            updateStatusPanelContent(data);
        }

        previousStatus = status;

        // Check for idle timeout warning
        if (config.idleTimeout > 0 && data.idleTimeRemaining > 0 && data.idleTimeRemaining <= 60) {
            showShutdownWarning(data.idleTimeRemaining);
        }
    }

    function updateStatusPanelContent(data) {
        var matlab = data.matlab || {};
        var status = matlab.status || "down";
        var licensing = data.licensing || {};

        // Update status badge
        var badges = document.querySelectorAll("#overlay-body .status-badge");
        if (badges.length > 0) {
            var badge = badges[0];
            badge.className = "value status-badge status-" + status;
            if (status === "up") badge.textContent = "Running";
            else if (status === "starting") badge.textContent = "Starting...";
            else if (status === "stopping") badge.textContent = "Stopping...";
            else if (status === "down") badge.textContent = "Not Running";
            else badge.textContent = status;
        }

        // Update licensing info
        var licEl = document.querySelector("#overlay-body .licensing-info");
        if (licEl) {
            var licType = licensing.type || "";
            if (licType === "mhlm") {
                licEl.textContent = "Online License (" + (licensing.email_addr || "") + ")";
            } else if (licType === "nlm") {
                licEl.textContent = "Network License (" + (licensing.conn_str || "") + ")";
            } else if (licType === "existing_license") {
                licEl.textContent = "Existing License";
            } else {
                licEl.textContent = "Not Configured";
            }
        }

        // Update error section
        var errorSection = document.querySelector("#overlay-body .error-section");
        var errData = data.error;
        if (errData && errData.message) {
            if (!errorSection) {
                errorSection = document.createElement("div");
                errorSection.className = "error-section";
                var statusSection = document.querySelector("#overlay-body .status-section");
                if (statusSection) statusSection.parentNode.insertBefore(errorSection, statusSection.nextSibling);
            }
            if (errorSection) errorSection.innerHTML = '<div class="error-banner">' + escapeHtml(errData.message) + '</div>';
        } else if (errorSection) {
            errorSection.remove();
        }

        // Update warning section
        var warningSection = document.querySelector("#overlay-body .warning-section");
        var warnings = data.warnings || [];
        if (warnings.length > 0) {
            if (!warningSection) {
                warningSection = document.createElement("div");
                warningSection.className = "warning-section";
                var after = document.querySelector("#overlay-body .error-section") || document.querySelector("#overlay-body .status-section");
                if (after) after.parentNode.insertBefore(warningSection, after.nextSibling);
            }
            if (warningSection) warningSection.innerHTML = warnings.map(function(w) { return '<div class="warning-banner">' + escapeHtml(w) + '</div>'; }).join("");
        } else if (warningSection) {
            warningSection.remove();
        }

        // Update controls based on MATLAB status
        var controlsEl = document.querySelector("#overlay-body .controls");
        if (controlsEl) {
            var licType = licensing.type || "";
            var buttons = "";
            if (!licType) {
                buttons += '<button class="btn btn-primary" onclick="showLicensing()">Configure License</button>';
            } else {
                if (status === "down") {
                    buttons += '<button class="btn btn-primary" onclick="startMatlab()">Start MATLAB</button>';
                }
                if (status === "up") {
                    buttons += '<button class="btn btn-primary" onclick="restartMatlab()">Restart MATLAB</button>';
                    buttons += '<button class="btn btn-danger" onclick="stopMatlab()">Stop MATLAB</button>';
                }
                if (status === "starting") {
                    buttons += '<button class="btn btn-danger" onclick="stopMatlab()" disabled>Stop MATLAB</button>';
                }
                if (status === "stopping") {
                    buttons += '<button class="btn btn-primary" onclick="restartMatlab()">Restart MATLAB</button>';
                }
                buttons += '<button class="btn btn-secondary" onclick="showLicensing()">Change License</button>';
                buttons += '<button class="btn btn-warning" onclick="removeLicense()">Sign Out</button>';
            }
            buttons += '<button class="btn btn-danger" onclick="confirmShutdown()">Shutdown</button>';
            controlsEl.innerHTML = buttons;
        }
    }

    function escapeHtml(str) {
        var div = document.createElement("div");
        div.appendChild(document.createTextNode(str));
        return div.innerHTML;
    }

    // --- Overlay management ---

    function showOverlay(templateId) {
        const overlay = document.getElementById("overlay");
        const body = document.getElementById("overlay-body");
        const tmpl = document.getElementById(templateId);
        body.innerHTML = tmpl.innerHTML;
        overlay.classList.remove("hidden");
        currentOverlay = templateId;
    }

    window.closeOverlay = function(e) {
        if (e && e.target !== document.getElementById("overlay")) return;
        // Don't allow dismissing auth or concurrent session dialogs by clicking backdrop
        if (currentOverlay === "tmpl-auth" || currentOverlay === "tmpl-concurrent-session" || currentOverlay === "tmpl-session-transferred") return;
        document.getElementById("overlay").classList.add("hidden");
        currentOverlay = "";
    };

    window.showStatusPanel = function() {
        // Re-render the status panel with fresh data from polling
        showOverlay("tmpl-status-panel");
    };

    window.showLicensing = function() {
        showOverlay("tmpl-licensing-panel");
    };

    // --- Trigger click ---

    function setupTrigger() {
        const trigger = document.getElementById("trigger");
        let isDragging = false;
        let startX, startY, origX, origY;

        trigger.addEventListener("mousedown", function(e) {
            isDragging = false;
            startX = e.clientX;
            startY = e.clientY;
            const rect = trigger.getBoundingClientRect();
            origX = rect.left;
            origY = rect.top;

            function onMove(e) {
                const dx = e.clientX - startX;
                const dy = e.clientY - startY;
                if (Math.abs(dx) > 3 || Math.abs(dy) > 3) {
                    isDragging = true;
                    trigger.style.left = (origX + dx) + "px";
                    trigger.style.top = (origY + dy) + "px";
                    trigger.style.right = "auto";
                    trigger.style.bottom = "auto";
                }
            }

            function onUp() {
                document.removeEventListener("mousemove", onMove);
                document.removeEventListener("mouseup", onUp);
                if (!isDragging) {
                    togglePanel();
                }
            }

            document.addEventListener("mousemove", onMove);
            document.addEventListener("mouseup", onUp);
        });

        trigger.addEventListener("dragstart", function(e) {
            e.preventDefault();
        });
    }

    function togglePanel() {
        const overlay = document.getElementById("overlay");
        if (overlay.classList.contains("hidden")) {
            showStatusPanel();
        } else {
            overlay.classList.add("hidden");
        }
    }

    // --- Session concurrency ---

    window.transferSession = function() {
        // Re-poll with TRANSFER_SESSION=true to take over the active session
        var params = new URLSearchParams({
            MWI_CLIENT_ID: clientId,
            IS_DESKTOP: "TRUE",
            TRANSFER_SESSION: "true"
        });
        api("GET", "/get_status?" + params.toString()).then(function(data) {
            if (data) {
                updateUI(data);
            }
        });
    };

    window.dismissConcurrentDialog = function() {
        document.getElementById("overlay").classList.add("hidden");
        currentOverlay = "";
    };

    // --- MATLAB controls ---

    window.startMatlab = function() {
        api("PUT", "/start_matlab", {});
        closeOverlay();
    };

    window.restartMatlab = function() {
        showConfirm("Restart MATLAB", "Are you sure you want to restart MATLAB? Any unsaved work will be lost.", function() {
            api("PUT", "/start_matlab", {});
            showStatusPanel();
        });
    };

    window.stopMatlab = function() {
        showConfirm("Stop MATLAB", "Are you sure you want to stop MATLAB?", function() {
            api("DELETE", "/stop_matlab");
            showStatusPanel();
        });
    };

    window.removeLicense = function() {
        showConfirm("Sign Out", "This will stop MATLAB and remove the license configuration.", function() {
            api("DELETE", "/set_licensing_info");
            showStatusPanel();
        });
    };

    window.confirmShutdown = function() {
        showConfirm("Shutdown", "This will stop MATLAB and shut down the proxy server.", function() {
            api("DELETE", "/shutdown_integration");
            document.body.innerHTML = "<div style='display:flex;align-items:center;justify-content:center;height:100vh;color:#888;font-size:1.2rem'>Server has been shut down.</div>";
        });
    };

    // --- Licensing ---

    window.setLicensing = function(type, extra) {
        const body = Object.assign({type: type}, extra);
        api("PUT", "/set_licensing_info", body).then(function(data) {
            if (data && !data.error) {
                showStatusPanel();
                // Auto-start MATLAB after licensing
                api("PUT", "/start_matlab", {});
            }
        });
    };

    window.submitNLM = function() {
        const conn = document.getElementById("nlm-conn").value.trim();
        if (!conn) return;
        setLicensing("nlm", {connectionString: conn});
    };

    // --- Tabs ---

    window.showTab = function(btn, tabId) {
        const tabs = btn.parentElement.querySelectorAll(".tab");
        tabs.forEach(t => t.classList.remove("active"));
        btn.classList.add("active");

        const panel = btn.closest(".panel");
        panel.querySelectorAll(".tab-content").forEach(tc => tc.classList.remove("active"));
        document.getElementById(tabId).classList.add("active");
    };

    // --- Confirmation dialog ---

    function showConfirm(title, message, onYes) {
        showOverlay("tmpl-confirm");
        document.getElementById("confirm-title").textContent = title;
        document.getElementById("confirm-message").textContent = message;
        document.getElementById("confirm-yes").onclick = function() {
            onYes();
        };
    }

    // --- Shutdown warning ---

    function showShutdownWarning(seconds) {
        if (shutdownCountdown) return;
        showOverlay("tmpl-shutdown-warning");
        let remaining = seconds;
        const el = document.getElementById("shutdown-countdown");
        shutdownCountdown = setInterval(function() {
            remaining--;
            if (el) el.textContent = remaining;
            if (remaining <= 0) {
                clearInterval(shutdownCountdown);
                shutdownCountdown = null;
            }
        }, 1000);
    }

    window.resetIdleTimer = function() {
        if (shutdownCountdown) {
            clearInterval(shutdownCountdown);
            shutdownCountdown = null;
        }
        // Any API call resets the idle timer server-side
        api("GET", "/get_status?MWI_CLIENT_ID=" + clientId);
        closeOverlay();
    };

    // --- MHLM embedded login flow ---
    // Mirrors the postMessage protocol from MHLM.jsx in the Python version.
    // Flow: iframe loads -> send 'init' nonce -> iframe replies 'nonce' -> send 'load' -> user logs in -> iframe sends 'login'

    let mhlmSourceId = Math.random().toString(36).substring(2, 15) + Math.random().toString(36).substring(2, 15);

    // Called when the MHLM login iframe finishes loading
    window.onMHLMIframeLoaded = function() {
        var loginFrame = document.getElementById("loginframe");
        if (!loginFrame || !loginFrame.contentWindow) return;

        var clientNonce = (Math.random() + '').substr(2);
        var noncePayload = {
            event: "init",
            clientTransactionId: clientNonce,
            transactionId: "",
            release: "",
            platform: "",
            clientString: "desktop-jupyter",
            clientID: "",
            locale: "",
            profileTier: "",
            showCreateAccount: false,
            showRememberMe: false,
            showLicenseField: false,
            licenseNo: "",
            cachedUsername: "",
            cachedRememberMe: false
        };
        loginFrame.contentWindow.postMessage(JSON.stringify(noncePayload), "*");
    };

    function setupMHLMListener() {
        window.addEventListener("message", function(event) {
            var mhlmOrigin = config.mhlmLoginOrigin || "https://login.mathworks.com";
            console.log("postMessage received", "origin:", event.origin, "expected:", mhlmOrigin, "data:", event.data);
            if (event.origin !== mhlmOrigin) return;

            var data;
            try {
                data = (typeof event.data === "string") ? JSON.parse(event.data) : event.data;
            } catch (e) {
                console.error("Failed to parse postMessage data:", e);
                return;
            }
            if (!data || !data.event) return;
            console.log("MHLM event:", data.event);

            if (data.event === "nonce") {
                // Step 2: Send 'load' event with the server nonce
                var loginFrame = document.getElementById("loginframe");
                if (!loginFrame || !loginFrame.contentWindow) return;

                var initPayload = {
                    event: "load",
                    clientTransactionId: data.clientTransactionId,
                    transactionId: data.transactionId,
                    release: "",
                    platform: "web",
                    clientString: "desktop-jupyter",
                    clientId: "",
                    sourceId: mhlmSourceId,
                    profileTier: "MINIMUM",
                    showCreateAccount: false,
                    showRememberMe: false,
                    showLicenseField: false,
                    entitlementId: "",
                    showPrivacyPolicy: true,
                    contextualText: "",
                    legalText: "",
                    cachedIdentifier: "",
                    cachedRememberMe: "",
                    token: "",
                    unauthorized: false
                };
                loginFrame.contentWindow.postMessage(JSON.stringify(initPayload), "*");

            } else if (data.event === "login") {
                // Step 3: User logged in, send credentials to our backend
                setLicensing("mhlm", {
                    token: data.token,
                    profileId: data.profileId,
                    emailAddress: data.emailAddress,
                    sourceId: mhlmSourceId,
                    matlabVersion: config.matlabVersion || ""
                });
            }
        });
    }

    // --- Terminal ---

    let term = null;
    let termSocket = null;
    let termFitAddon = null;
    let termState = "closed"; // closed | open | minimized

    function initTerminal() {
        var drawer = document.getElementById("terminal-drawer");
        var container = document.getElementById("terminal-container");
        var toggleBtn = document.getElementById("terminal-toggle");

        // Set initial height from localStorage or default 40%
        var savedHeight = localStorage.getItem("terminalHeight");
        var height = savedHeight ? parseInt(savedHeight, 10) : Math.round(window.innerHeight * 0.4);
        drawer.style.height = height + "px";

        // Create xterm instance
        term = new window.Terminal({
            cursorBlink: true,
            fontSize: 14,
            fontFamily: "'Cascadia Code', 'Fira Code', 'Source Code Pro', Menlo, monospace",
            theme: {
                background: "#1e1e1e",
                foreground: "#d4d4d4",
                cursor: "#aeafad"
            }
        });

        termFitAddon = new window.FitAddon.FitAddon();
        term.loadAddon(termFitAddon);
        term.open(container);

        // Connect WebSocket
        var proto = window.location.protocol === "https:" ? "wss:" : "ws:";
        var wsUrl = proto + "//" + window.location.host + config.baseURL + "/terminal/ws";
        if (authToken) {
            wsUrl += "?mwi-auth-token=" + encodeURIComponent(authToken);
        }
        termSocket = new WebSocket(wsUrl);

        termSocket.onopen = function() {
            termFitAddon.fit();
            // Send initial size
            var dims = termFitAddon.proposeDimensions();
            if (dims) {
                termSocket.send(new Blob([JSON.stringify({cols: dims.cols, rows: dims.rows})]));
            }
        };

        termSocket.onmessage = function(e) {
            if (typeof e.data === "string") {
                term.write(e.data);
            } else if (e.data instanceof Blob) {
                e.data.text().then(function(text) { term.write(text); });
            }
        };

        termSocket.onclose = function() {
            term.write("\r\n\x1b[90m[Session ended]\x1b[0m\r\n");
            toggleBtn.classList.remove("terminal-active");
        };

        // Terminal input → WebSocket
        term.onData(function(data) {
            if (termSocket && termSocket.readyState === WebSocket.OPEN) {
                termSocket.send(data);
            }
        });

        // Handle resize
        var resizeObserver = new ResizeObserver(function() {
            if (termState === "open" && termFitAddon) {
                termFitAddon.fit();
                if (termSocket && termSocket.readyState === WebSocket.OPEN) {
                    var dims = termFitAddon.proposeDimensions();
                    if (dims) {
                        termSocket.send(new Blob([JSON.stringify({cols: dims.cols, rows: dims.rows})]));
                    }
                }
            }
        });
        resizeObserver.observe(container);

        toggleBtn.classList.add("terminal-active");
    }

    function destroyTerminal() {
        if (termSocket) {
            termSocket.close();
            termSocket = null;
        }
        if (term) {
            term.dispose();
            term = null;
        }
        termFitAddon = null;
        var container = document.getElementById("terminal-container");
        container.innerHTML = "";
        document.getElementById("terminal-toggle").classList.remove("terminal-active");
    }

    function setTermState(state) {
        var drawer = document.getElementById("terminal-drawer");
        var frameContainer = document.getElementById("matlab-frame-container");

        termState = state;

        if (state === "closed") {
            drawer.classList.add("terminal-closed");
            drawer.classList.remove("terminal-minimized");
            frameContainer.style.height = "100%";
            destroyTerminal();
        } else if (state === "minimized") {
            drawer.classList.remove("terminal-closed");
            drawer.classList.add("terminal-minimized");
            frameContainer.style.height = "calc(100% - 36px)";
        } else {
            // open
            drawer.classList.remove("terminal-closed");
            drawer.classList.remove("terminal-minimized");
            var height = parseInt(drawer.style.height, 10) || Math.round(window.innerHeight * 0.4);
            frameContainer.style.height = "calc(100% - " + height + "px)";
            if (termFitAddon) {
                setTimeout(function() { termFitAddon.fit(); }, 50);
            }
        }
    }

    function toggleTerminal() {
        if (termState === "closed") {
            setTermState("open");
            initTerminal();
        } else if (termState === "minimized") {
            setTermState("open");
        } else {
            setTermState("minimized");
        }
    }

    function setupTerminalUI() {
        var toggleBtn = document.getElementById("terminal-toggle");
        var minimizeBtn = document.getElementById("terminal-minimize");
        var closeBtn = document.getElementById("terminal-close");
        var divider = document.getElementById("terminal-divider");
        var drawer = document.getElementById("terminal-drawer");
        var frameContainer = document.getElementById("matlab-frame-container");

        toggleBtn.addEventListener("click", toggleTerminal);

        minimizeBtn.addEventListener("click", function() {
            if (termState === "minimized") {
                setTermState("open");
            } else {
                setTermState("minimized");
            }
        });

        closeBtn.addEventListener("click", function() {
            setTermState("closed");
        });

        // Keyboard shortcut: Ctrl+`
        document.addEventListener("keydown", function(e) {
            if (e.ctrlKey && e.key === "`") {
                e.preventDefault();
                toggleTerminal();
            }
        });

        // Draggable divider for resizing
        divider.addEventListener("mousedown", function(e) {
            e.preventDefault();
            var startY = e.clientY;
            var startHeight = parseInt(drawer.style.height, 10) || Math.round(window.innerHeight * 0.4);

            // Disable pointer events on iframes so they don't steal mousemove during drag
            var matlabFrame = document.getElementById("matlab-frame");
            if (matlabFrame) matlabFrame.style.pointerEvents = "none";

            function onMouseMove(e) {
                var newHeight = startHeight + (startY - e.clientY);
                var minH = 100;
                var maxH = window.innerHeight - 80;
                newHeight = Math.max(minH, Math.min(maxH, newHeight));
                drawer.style.height = newHeight + "px";
                frameContainer.style.height = "calc(100% - " + newHeight + "px)";
            }

            function onMouseUp() {
                document.removeEventListener("mousemove", onMouseMove);
                document.removeEventListener("mouseup", onMouseUp);
                // Re-enable pointer events on the iframe
                if (matlabFrame) matlabFrame.style.pointerEvents = "";
                localStorage.setItem("terminalHeight", parseInt(drawer.style.height, 10));
                if (termFitAddon) termFitAddon.fit();
                // Send updated size
                if (termSocket && termSocket.readyState === WebSocket.OPEN) {
                    var dims = termFitAddon.proposeDimensions();
                    if (dims) {
                        termSocket.send(new Blob([JSON.stringify({cols: dims.cols, rows: dims.rows})]));
                    }
                }
            }

            document.addEventListener("mousemove", onMouseMove);
            document.addEventListener("mouseup", onMouseUp);
        });
    }

    // --- Tab close cleanup ---

    function setupBeaconCleanup() {
        window.addEventListener("beforeunload", function() {
            const url = config.baseURL + "/clear_client_id";
            navigator.sendBeacon(url, JSON.stringify({clientId: clientId}));
        });
    }

    // --- Authentication ---

    function checkAuth(token) {
        var opts = {
            method: "POST",
            credentials: "same-origin",
            headers: {}
        };
        if (token) {
            opts.headers["mwi-auth-token"] = token;
        }
        var url = config.baseURL + "/authenticate";
        return fetch(url, opts).then(function(r) {
            console.log("AUTH response status:", r.status, "has set-cookie:", r.headers.has("set-cookie"));
            return r.json();
        }).then(function(data) {
            console.log("AUTH result:", JSON.stringify(data));
            if (data && data.authentication && data.authentication.status === true) {
                return true;
            }
            return false;
        }).catch(function(err) {
            console.error("AUTH error:", err);
            return false;
        });
    }

    function showAuthScreen() {
        var overlay = document.getElementById("overlay");
        var body = document.getElementById("overlay-body");
        var tmpl = document.getElementById("tmpl-auth");
        body.innerHTML = tmpl.innerHTML;
        overlay.classList.remove("hidden");
        currentOverlay = "tmpl-auth";

        var input = document.getElementById("auth-token-input");
        if (input) input.focus();
    }

    window.submitAuthToken = function() {
        var input = document.getElementById("auth-token-input");
        var errorEl = document.getElementById("auth-error");
        if (!input) return;

        var token = input.value.trim();
        if (!token) return;

        if (errorEl) errorEl.textContent = "";

        checkAuth(token).then(function(valid) {
            if (valid) {
                authToken = token;
                document.getElementById("overlay").classList.add("hidden");
                currentOverlay = "";
                // Strip token from URL for cleanliness
                window.history.replaceState(null, "", config.baseURL || "/");
                startApp();
            } else {
                if (errorEl) {
                    errorEl.textContent = "Invalid token. Please try again.";
                    errorEl.style.display = "block";
                }
            }
        });
    };

    function startApp() {
        // Remove any stale iframe that was rendered server-side without auth
        var staleFrame = document.getElementById("matlab-frame");
        if (staleFrame) {
            var container = document.getElementById("matlab-frame-container");
            container.innerHTML = '<div id="matlab-placeholder"><img src="' + config.baseURL + '/static/icons/matlab-logo.svg" alt="MATLAB" class="placeholder-logo" onerror="this.style.display=\'none\'"></div>';
        }

        setupTrigger();
        if (config.terminalEnabled) setupTerminalUI();
        setupMHLMListener();
        setupBeaconCleanup();

        pollStatus();
        statusPollTimer = setInterval(pollStatus, 1000);
    }

    // --- Init ---

    window.init = function() {
        config = window.MATLAB_PROXY || {};

        // Extract auth token from URL query parameter
        var params = new URLSearchParams(window.location.search);
        var urlToken = params.get("mwi-auth-token") || "";

        if (!config.authEnabled) {
            // Auth disabled, start directly
            startApp();
            return;
        }

        if (urlToken) {
            // Token in URL — validate and proceed
            checkAuth(urlToken).then(function(valid) {
                if (valid) {
                    authToken = urlToken;
                    window.history.replaceState(null, "", config.baseURL || "/");
                    startApp();
                } else {
                    showAuthScreen();
                }
            });
        } else {
            // No token in URL — check if session cookie is valid
            checkAuth("").then(function(valid) {
                if (valid) {
                    startApp();
                } else {
                    showAuthScreen();
                }
            });
        }
    };
})();
