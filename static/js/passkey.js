// Passkey (WebAuthn) registration and discoverable login.
//
// The server speaks the standard WebAuthn JSON dialect where binary fields are
// base64url-encoded strings. The browser APIs want ArrayBuffers, so this script
// converts options on the way in and responses on the way out.
(function () {
  "use strict";

  if (!window.PublicKeyCredential || !navigator.credentials) {
    document.querySelectorAll("[data-passkey-unsupported]").forEach(function (el) {
      el.hidden = false;
    });
    document.querySelectorAll("[data-passkey-requires-support]").forEach(function (el) {
      el.disabled = true;
    });
    return;
  }

  function b64urlToBuf(value) {
    var padded = value.replace(/-/g, "+").replace(/_/g, "/");
    while (padded.length % 4) {
      padded += "=";
    }
    var binary = atob(padded);
    var bytes = new Uint8Array(binary.length);
    for (var i = 0; i < binary.length; i++) {
      bytes[i] = binary.charCodeAt(i);
    }
    return bytes.buffer;
  }

  function bufToB64url(buffer) {
    var bytes = new Uint8Array(buffer);
    var binary = "";
    for (var i = 0; i < bytes.length; i++) {
      binary += String.fromCharCode(bytes[i]);
    }
    return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }

  function decodeCreationOptions(options) {
    var publicKey = options.publicKey;
    publicKey.challenge = b64urlToBuf(publicKey.challenge);
    publicKey.user.id = b64urlToBuf(publicKey.user.id);
    (publicKey.excludeCredentials || []).forEach(function (credential) {
      credential.id = b64urlToBuf(credential.id);
    });
    return publicKey;
  }

  function decodeRequestOptions(options) {
    var publicKey = options.publicKey;
    publicKey.challenge = b64urlToBuf(publicKey.challenge);
    (publicKey.allowCredentials || []).forEach(function (credential) {
      credential.id = b64urlToBuf(credential.id);
    });
    return publicKey;
  }

  function encodeAttestation(credential) {
    var response = credential.response;
    return {
      id: credential.id,
      rawId: bufToB64url(credential.rawId),
      type: credential.type,
      clientExtensionResults: credential.getClientExtensionResults(),
      authenticatorAttachment: credential.authenticatorAttachment || undefined,
      response: {
        clientDataJSON: bufToB64url(response.clientDataJSON),
        attestationObject: bufToB64url(response.attestationObject),
        transports: response.getTransports ? response.getTransports() : [],
      },
    };
  }

  function encodeAssertion(credential) {
    var response = credential.response;
    return {
      id: credential.id,
      rawId: bufToB64url(credential.rawId),
      type: credential.type,
      clientExtensionResults: credential.getClientExtensionResults(),
      authenticatorAttachment: credential.authenticatorAttachment || undefined,
      response: {
        clientDataJSON: bufToB64url(response.clientDataJSON),
        authenticatorData: bufToB64url(response.authenticatorData),
        signature: bufToB64url(response.signature),
        userHandle: response.userHandle ? bufToB64url(response.userHandle) : undefined,
      },
    };
  }

  function postJSON(url, csrf, body) {
    return fetch(url, {
      method: "POST",
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": csrf,
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  }

  async function readError(response, fallback) {
    try {
      var data = await response.json();
      if (data && data.error) {
        return data.error;
      }
    } catch (e) {
      /* ignore */
    }
    return fallback;
  }

  function showError(el, message) {
    if (!el) {
      return;
    }
    el.textContent = message;
    el.hidden = false;
  }

  function clearError(el) {
    if (!el) {
      return;
    }
    el.textContent = "";
    el.hidden = true;
  }

  // Registration (logged-in user adds a passkey).
  function setupRegistration() {
    var form = document.getElementById("passkey-register-form");
    if (!form) {
      return;
    }
    var button = document.getElementById("passkey-register-button");
    var nameInput = document.getElementById("passkey-name");
    var errorEl = document.getElementById("passkey-error");
    var beginURL = button.getAttribute("data-begin-url");
    var finishURL = button.getAttribute("data-finish-url");
    var csrf = button.getAttribute("data-csrf");

    form.addEventListener("submit", async function (event) {
      event.preventDefault();
      clearError(errorEl);
      button.setAttribute("aria-busy", "true");
      button.disabled = true;
      try {
        var beginResponse = await postJSON(beginURL, csrf);
        if (!beginResponse.ok) {
          showError(errorEl, await readError(beginResponse, "Could not start passkey registration."));
          return;
        }
        var options = decodeCreationOptions(await beginResponse.json());
        var credential = await navigator.credentials.create({ publicKey: options });
        if (!credential) {
          showError(errorEl, "Passkey registration was cancelled.");
          return;
        }
        var name = nameInput && nameInput.value ? nameInput.value.trim() : "";
        var finishResponse = await postJSON(
          finishURL + "?name=" + encodeURIComponent(name),
          csrf,
          encodeAttestation(credential)
        );
        if (!finishResponse.ok) {
          showError(errorEl, await readError(finishResponse, "Could not save your passkey."));
          return;
        }
        var result = await finishResponse.json();
        window.location.assign(result.redirect || window.location.pathname);
      } catch (err) {
        if (err && err.name === "NotAllowedError") {
          showError(errorEl, "Passkey registration was cancelled or timed out.");
        } else if (err && err.name === "InvalidStateError") {
          showError(errorEl, "This device already has a passkey for your account.");
        } else {
          showError(errorEl, "Something went wrong setting up your passkey.");
        }
      } finally {
        button.removeAttribute("aria-busy");
        button.disabled = false;
      }
    });
  }

  // Discoverable login (anonymous user signs in with a passkey).
  function setupLogin() {
    var container = document.getElementById("passkey-login");
    if (!container) {
      return;
    }
    var button = document.getElementById("passkey-login-button");
    var errorEl = document.getElementById("passkey-login-error");
    var beginURL = container.getAttribute("data-begin-url");
    var finishURL = container.getAttribute("data-finish-url");
    var csrf = container.getAttribute("data-csrf");
    var next = container.getAttribute("data-next") || "";

    async function authenticate(mediation) {
      clearError(errorEl);
      var beginResponse = await postJSON(beginURL, csrf);
      if (!beginResponse.ok) {
        if (mediation !== "conditional") {
          showError(errorEl, await readError(beginResponse, "Could not start passkey sign-in."));
        }
        return;
      }
      var options = decodeRequestOptions(await beginResponse.json());
      var request = { publicKey: options };
      if (mediation === "conditional") {
        request.mediation = "conditional";
      }
      var credential = await navigator.credentials.get(request);
      if (!credential) {
        return;
      }
      var finishURLWithNext = next ? finishURL + "?next=" + encodeURIComponent(next) : finishURL;
      var finishResponse = await postJSON(finishURLWithNext, csrf, encodeAssertion(credential));
      if (!finishResponse.ok) {
        showError(errorEl, await readError(finishResponse, "That passkey could not be verified."));
        return;
      }
      var result = await finishResponse.json();
      window.location.assign(result.redirect || "/account");
    }

    if (button) {
      button.addEventListener("click", function () {
        button.setAttribute("aria-busy", "true");
        button.disabled = true;
        authenticate("optional")
          .catch(function (err) {
            if (!err || err.name !== "NotAllowedError") {
              showError(errorEl, "Something went wrong signing in with your passkey.");
            }
          })
          .finally(function () {
            button.removeAttribute("aria-busy");
            button.disabled = false;
          });
      });
    }

    // Conditional UI: offer passkeys via browser autofill when supported.
    if (PublicKeyCredential.isConditionalMediationAvailable) {
      PublicKeyCredential.isConditionalMediationAvailable().then(function (available) {
        if (available) {
          authenticate("conditional").catch(function () {
            /* autofill failures are non-fatal */
          });
        }
      });
    }
  }

  setupRegistration();
  setupLogin();
})();
