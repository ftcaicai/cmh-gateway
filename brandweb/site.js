// Brand storefront sign-in. The access token lives only in this page's memory;
// the refresh credential is an HttpOnly cookie this script cannot read.
(function () {
  "use strict";

  var page = document.body.dataset.page;
  var status = document.getElementById("status");

  function show(message) {
    if (!status) return;
    status.textContent = message;
    status.hidden = false;
  }

  function request(method, path, body, accessToken) {
    var headers = {};
    if (body) headers["Content-Type"] = "application/json";
    if (accessToken) headers["Authorization"] = "Bearer " + accessToken;
    return fetch(path, {
      method: method,
      headers: headers,
      body: body ? JSON.stringify(body) : undefined,
      credentials: "same-origin",
      cache: "no-store",
    }).then(function (response) {
      return response.json().catch(function () {
        return { success: false };
      });
    });
  }

  function base64(buffer) {
    var bytes = new Uint8Array(buffer);
    var binary = "";
    for (var i = 0; i < bytes.length; i += 1) binary += String.fromCharCode(bytes[i]);
    return btoa(binary);
  }

  // Mirrors the gateway's "v2" envelope: a random AES-256-GCM key wrapped with
  // RSA-OAEP(SHA-256), so the password is not sent in clear inside TLS.
  function encryptPassword(password, key) {
    var encoder = new TextEncoder();
    var der = Uint8Array.from(
      atob(key.public_key.replace(/-----[A-Z ]+-----/g, "").replace(/\s+/g, "")),
      function (ch) {
        return ch.charCodeAt(0);
      }
    );
    var secret = crypto.getRandomValues(new Uint8Array(32));
    var nonce = crypto.getRandomValues(new Uint8Array(12));
    return Promise.all([
      crypto.subtle.importKey("spki", der, { name: "RSA-OAEP", hash: "SHA-256" }, false, ["encrypt"]),
      crypto.subtle.importKey("raw", secret, "AES-GCM", false, ["encrypt"]),
    ])
      .then(function (keys) {
        return Promise.all([
          crypto.subtle.encrypt({ name: "RSA-OAEP", label: encoder.encode("password-v2") }, keys[0], secret),
          crypto.subtle.encrypt(
            { name: "AES-GCM", iv: nonce, additionalData: encoder.encode("password-v2:" + key.kid) },
            keys[1],
            encoder.encode(password)
          ),
        ]);
      })
      .then(function (parts) {
        return ["v2", base64(parts[0]), base64(nonce), base64(parts[1])].join(".");
      });
  }

  function hasSessionHint() {
    return document.cookie.split("; ").indexOf("new_api_has_session=1") !== -1;
  }

  function refresh() {
    if (!hasSessionHint()) return Promise.resolve(null);
    return request("POST", "/api/user/auth/refresh").then(function (result) {
      return result.success && result.data ? result.data : null;
    });
  }

  // Signed-in visitors get a link to their account instead of "sign in".
  var navAccount = document.getElementById("nav-account");
  if (navAccount && hasSessionHint()) {
    navAccount.textContent = "我的账号";
    navAccount.href = "/account";
  }

  if (page === "pricing") {
    var list = document.getElementById("models");
    var count = document.getElementById("model-count");
    var models = [];
    var element = function (tag, className, text) {
      var node = document.createElement(tag);
      if (className) node.className = className;
      if (text) node.textContent = text;
      return node;
    };
    var money = function (symbol, value) {
      return symbol + Number(value.toFixed(6)).toString();
    };
    var render = function () {
      var keyword = document.getElementById("model-search").value.trim().toLowerCase();
      var shown = models.filter(function (model) {
        return !keyword || (model.name + " " + model.vendor).toLowerCase().indexOf(keyword) !== -1;
      });
      list.replaceChildren.apply(
        list,
        shown.map(function (model) {
          var card = element("article", "model");
          card.appendChild(element("div", "vendor", model.vendor || "模型"));
          card.appendChild(element("h2", "", model.name));
          if (model.description) card.appendChild(element("p", "", model.description));
          var tags = element("div", "tags");
          model.endpoints.forEach(function (endpoint) {
            tags.appendChild(element("span", "", endpoint));
          });
          card.appendChild(tags);
          var price = element("div", "price");
          if (model.billing === "token") {
            price.appendChild(element("div", "", "输入 " + money(model.currency, model.input) + " / 百万 token"));
            price.appendChild(element("div", "", "输出 " + money(model.currency, model.output) + " / 百万 token"));
          } else if (model.billing === "call") {
            price.appendChild(element("div", "", money(model.currency, model.call) + " / 次"));
          } else {
            price.appendChild(element("div", "", "按模型计费规则计价"));
          }
          card.appendChild(price);
          return card;
        })
      );
      count.textContent = "显示 " + shown.length + " / " + models.length + " 个模型";
    };
    request("GET", "/api/ext/brand/pricing").then(function (result) {
      if (!result.success) {
        show("暂时无法加载模型，请稍后重试。");
        return;
      }
      models = result.data;
      status.hidden = models.length > 0;
      if (!models.length) show("暂无可用模型。");
      document.getElementById("model-search").addEventListener("input", render);
      render();
    });
    return;
  }

  if (page === "account") {
    refresh().then(function (session) {
      if (!session) {
        location.replace("/login");
        return;
      }
      return request("GET", "/api/ext/brand/self", null, session.access_token).then(function (result) {
        if (!result.success) {
          show("暂时无法加载账号资料，请稍后重试。");
          return;
        }
        document.getElementById("profile-username").textContent = result.data.username;
        document.getElementById("profile-display-name").textContent = result.data.display_name || "—";
        document.getElementById("profile-email").textContent = result.data.email || "—";
        document.getElementById("profile-site").textContent = result.data.site_name;
        document.getElementById("profile").hidden = false;
        status.hidden = true;
        var logout = document.getElementById("logout");
        logout.hidden = false;
        logout.addEventListener("click", function () {
          logout.disabled = true;
          request("POST", "/api/user/auth/logout", null, session.access_token).then(function () {
            location.replace("/login");
          });
        });
      });
    });
    return;
  }

  var loginForm = document.getElementById("login-form");
  var verifyForm = document.getElementById("verify-form");
  if (!loginForm) return;
  var flowToken = "";

  // Cloudflare Turnstile, only when the main site requires it for sign-in.
  // The official login middleware reads the token from the query string.
  var turnstileBox = document.getElementById("turnstile");
  var turnstileToken = "";
  var turnstileWidget = null;
  window.cmhTurnstileReady = function () {
    if (!turnstileBox || !window.turnstile) return;
    turnstileWidget = window.turnstile.render(turnstileBox, {
      sitekey: turnstileBox.dataset.sitekey,
      callback: function (token) {
        turnstileToken = token;
      },
      "expired-callback": function () {
        turnstileToken = "";
      },
      "error-callback": function () {
        turnstileToken = "";
      },
    });
  };
  function loginPath() {
    if (!turnstileBox) return "/api/user/login";
    return "/api/user/login?turnstile=" + encodeURIComponent(turnstileToken);
  }
  function resetTurnstile() {
    if (turnstileWidget === null || !window.turnstile) return;
    turnstileToken = "";
    window.turnstile.reset(turnstileWidget);
  }

  refresh().then(function (session) {
    if (session) location.replace("/account");
  });

  function finish(result, failure) {
    if (!result.success || !result.data) {
      // Only this script's own messages are shown; server text never is.
      show(result.message === "请先完成人机验证。" ? result.message : failure);
      return;
    }
    if (result.data.require_verification) {
      flowToken = result.data.flow_token;
      loginForm.hidden = true;
      verifyForm.hidden = false;
      status.hidden = true;
      document.getElementById("code").focus();
      return;
    }
    location.replace("/account");
  }

  function submit(form, send, failure) {
    form.addEventListener("submit", function (event) {
      event.preventDefault();
      var button = form.querySelector("button");
      button.disabled = true;
      status.hidden = true;
      send()
        .then(function (result) {
          finish(result, failure);
        })
        .catch(function () {
          show("网络异常，请稍后重试。");
        })
        .then(function () {
          button.disabled = false;
        });
    });
  }

  submit(
    loginForm,
    function () {
      var username = document.getElementById("username").value.trim();
      var password = document.getElementById("password").value;
      if (!username || !password) return Promise.resolve({ success: false });
      if (turnstileBox && !turnstileToken) {
        return Promise.resolve({ success: false, message: "请先完成人机验证。" });
      }
      var path = loginPath();
      return request("GET", "/api/user/login/encryption-key")
        .then(function (key) {
          if (!key.success || !key.data) return { success: false };
          if (!key.data.enabled) {
            return request("POST", path, { username: username, password: password });
          }
          return encryptPassword(password, key.data).then(function (encrypted) {
            return request("POST", path, {
              username: username,
              password_encrypted: encrypted,
              encryption_key_id: key.data.kid,
            });
          });
        })
        .then(function (result) {
          // A Turnstile token is single-use; a new attempt needs a new one.
          resetTurnstile();
          return result;
        });
    },
    "账号或密码不正确，或该账号暂时无法在本站登录。"
  );

  submit(
    verifyForm,
    function () {
      var code = document.getElementById("code").value.trim();
      if (!code) return Promise.resolve({ success: false });
      return request("POST", "/api/user/login/verify", { flow_token: flowToken, method: "2fa", code: code });
    },
    "验证码不正确或已过期，请重试；多次失败后请重新登录。"
  );
})();
