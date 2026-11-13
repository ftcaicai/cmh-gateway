import assert from "node:assert/strict";
import { beforeEach, test } from "node:test";

const { default: axios } = await import("axios");
let refreshHandler;
const refreshCalls = [];
axios.defaults.adapter = async (config) => {
  refreshCalls.push(config);
  const result = await refreshHandler(config);
  return { config, headers: {}, statusText: "", ...result };
};
const { api, ApiError, officialLoginUrl } = await import("../src/lib/api.ts");
const { applyAuthBundle, clearAuthentication } = await import("@/lib/auth-session");
const { useAuthStore } = await import("@/stores/auth-store");
const { sanitizeAuthRedirect } = await import("@/features/auth/lib/auth-redirect");

function bundle(
  token = "test-access",
  sid = "test-session",
  expires = Math.floor(Date.now() / 1000) + 3600,
) {
  return {
    access_token: token,
    token_type: "Bearer",
    access_expires_at: expires,
    user: { id: 1, username: "alice", role: 1 },
    session: {
      sid,
      current: true,
      login_method: "password",
      ip: "",
      user_agent: "",
      created_at: 1,
      last_active_at: 1,
      expires_at: expires,
    },
  };
}
function refreshed(value = bundle()) {
  return { status: 200, data: { success: true, data: value } };
}
function selfResponse() {
  return Response.json({ success: true, data: { id: 1, username: "alice", role: 1 } });
}

beforeEach(() => {
  clearAuthentication(false, "idle");
  refreshCalls.length = 0;
  refreshHandler = async () => refreshed();
});

test("cold self request exchanges cookie via official refresh then sends Bearer and no-store headers", async (t) => {
  const calls = [];
  t.mock.method(globalThis, "fetch", async (path, options) => {
    calls.push({ path, options });
    return selfResponse();
  });
  assert.equal((await api.self()).username, "alice");
  assert.equal(refreshCalls.length, 1);
  assert.equal(refreshCalls[0].url, "/api/user/auth/refresh");
  assert.equal(refreshCalls[0].method, "post");
  assert.equal(refreshCalls[0].withCredentials, true);
  assert.equal(refreshCalls[0].headers.get("Cache-Control"), "no-cache, no-store");
  assert.equal(calls[0].path, "/api/user/self");
  assert.equal(calls[0].options.headers.Authorization, "Bearer test-access");
  assert.equal(calls[0].options.cache, "no-store");
  assert.equal(calls[0].options.credentials, "same-origin");
  assert.equal(calls[0].options.redirect, "error");
});

test("simultaneous cold reads share the official refresh flight", async (t) => {
  let release;
  refreshHandler = () =>
    new Promise((resolve) => {
      release = () => resolve(refreshed());
    });
  t.mock.method(globalThis, "fetch", async () => selfResponse());
  const first = api.self();
  const second = api.self();
  assert.equal(refreshCalls.length, 1);
  release();
  await Promise.all([first, second]);
  assert.equal(refreshCalls.length, 1);
});

test("expired access refreshes with expected official session header before self request", async (t) => {
  applyAuthBundle(bundle("expired", "test-session", 1), false);
  t.mock.method(globalThis, "fetch", async (_path, options) => {
    assert.equal(options.headers.Authorization, "Bearer test-access");
    return selfResponse();
  });
  await api.self();
  assert.equal(refreshCalls[0].headers.get("X-Auth-Session"), "test-session");
});

test("self 401 refreshes once and retries only the read with rotated access token", async (t) => {
  applyAuthBundle(bundle("old"), false);
  refreshHandler = async () => refreshed(bundle("rotated"));
  const tokens = [];
  t.mock.method(globalThis, "fetch", async (_path, options) => {
    tokens.push(options.headers.Authorization);
    return tokens.length === 1 ? new Response("", { status: 401 }) : selfResponse();
  });
  await api.self();
  assert.deepEqual(tokens, ["Bearer old", "Bearer rotated"]);
  assert.equal(refreshCalls.length, 1);
  assert.equal(refreshCalls[0].headers.get("X-Auth-Session"), "test-session");
});

test("repeated protected 401 clears memory and stops replaying", async (t) => {
  applyAuthBundle(bundle(), false);
  const fetchMock = t.mock.method(
    globalThis,
    "fetch",
    async () => new Response("", { status: 401 }),
  );
  await assert.rejects(api.self(), (error) => error instanceof ApiError && error.status === 401);
  assert.equal(fetchMock.mock.callCount(), 2);
  assert.equal(refreshCalls.length, 1);
  assert.equal(useAuthStore.getState().auth.accessToken, null);
});

test("refresh 401 is anonymous and never calls cookie-only self", async (t) => {
  refreshHandler = async () => ({ status: 401, data: { success: false } });
  const fetchMock = t.mock.method(globalThis, "fetch", async () => selfResponse());
  await assert.rejects(api.self(), (error) => error.status === 401);
  assert.equal(fetchMock.mock.callCount(), 0);
});

for (const status of [429, 503]) {
  test(`refresh ${status} is transient rather than a login redirect`, async (t) => {
    refreshHandler = async () => ({
      status,
      data: { success: false, message: "untrusted-secret" },
    });
    const fetchMock = t.mock.method(globalThis, "fetch", async () => selfResponse());
    await assert.rejects(
      api.self(),
      (error) => error.status === 503 && !error.message.includes("untrusted-secret"),
    );
    assert.equal(fetchMock.mock.callCount(), 0);
  });
}

test("malformed refresh is not accepted as an authenticated bundle", async (t) => {
  refreshHandler = async () => ({
    status: 200,
    data: { success: true, data: { access_token: "test-only" } },
  });
  const fetchMock = t.mock.method(globalThis, "fetch", async () => selfResponse());
  await assert.rejects(api.self(), (error) => error.status === 503);
  assert.equal(fetchMock.mock.callCount(), 0);
});

test("official session mismatch recovery never replays a read under a different account session", async (t) => {
  applyAuthBundle(bundle(), false);
  refreshHandler = async () => {
    if (refreshCalls.length === 1) return { status: 409, data: { code: "AUTH_SESSION_MISMATCH" } };
    return refreshed(bundle("other", "different-session"));
  };
  const fetchMock = t.mock.method(
    globalThis,
    "fetch",
    async () => new Response("", { status: 401 }),
  );
  await assert.rejects(api.self(), (error) => error.status === 409);
  assert.equal(fetchMock.mock.callCount(), 1);
  assert.equal(refreshCalls.length, 2);
  assert.equal(refreshCalls[0].headers.get("X-Auth-Session"), "test-session");
  assert.equal(refreshCalls[1].headers.get("X-Auth-Session"), undefined);
});

test("sign-out during pending self discards the identity response", async (t) => {
  applyAuthBundle(bundle(), false);
  t.mock.method(globalThis, "fetch", async () => {
    clearAuthentication(false);
    return selfResponse();
  });
  await assert.rejects(api.self(), (error) => error.status === 409);
});

test("sign-out during self body decoding discards the decoded identity", async (t) => {
  applyAuthBundle(bundle(), false);
  t.mock.method(globalThis, "fetch", async () => ({
    ok: true,
    status: 200,
    json: async () => {
      clearAuthentication(false);
      return { success: true, data: { username: "alice" } };
    },
  }));
  await assert.rejects(api.self(), (error) => error.status === 409);
});

test("public status neither refreshes nor attaches access credentials", async (t) => {
  applyAuthBundle(bundle(), false);
  t.mock.method(globalThis, "fetch", async (_path, options) => {
    assert.equal(options.headers.Authorization, undefined);
    return Response.json({ success: true, data: {} });
  });
  await api.status();
  assert.equal(refreshCalls.length, 0);
});

test("official refresh race retries with bounded delays and does not call self before success", async (t) => {
  const delays = [];
  t.mock.method(globalThis, "setTimeout", (callback, delay) => {
    delays.push(delay);
    callback();
    return 0;
  });
  refreshHandler = async () =>
    refreshCalls.length < 3 ? { status: 409, data: { code: "AUTH_REFRESH_RACE" } } : refreshed();
  t.mock.method(globalThis, "fetch", async () => selfResponse());
  await api.self();
  assert.deepEqual(delays, [80, 200]);
  assert.equal(refreshCalls.length, 3);
});

test("exhausted refresh race stays recoverable rather than redirecting or issuing cookie-only self", async (t) => {
  t.mock.method(globalThis, "setTimeout", (callback) => {
    callback();
    return 0;
  });
  refreshHandler = async () => ({ status: 409, data: { code: "AUTH_REFRESH_RACE" } });
  const fetchMock = t.mock.method(globalThis, "fetch", async () => selfResponse());
  await assert.rejects(api.self(), (error) => error.status === 503);
  assert.equal(refreshCalls.length, 4);
  assert.equal(fetchMock.mock.callCount(), 0);
});

test("refresh network failure stays recoverable without disclosing error details", async (t) => {
  refreshHandler = async () => {
    throw new Error("test-secret");
  };
  const fetchMock = t.mock.method(globalThis, "fetch", async () => selfResponse());
  await assert.rejects(
    api.self(),
    (error) => error.status === 503 && !error.message.includes("test-secret"),
  );
  assert.equal(fetchMock.mock.callCount(), 0);
});

test("successful self reuse never reads or writes usable tokens to web storage", async (t) => {
  const storage = {
    getItem() {
      throw new Error("storage read");
    },
    setItem() {
      throw new Error("storage write");
    },
  };
  globalThis.localStorage = storage;
  globalThis.sessionStorage = storage;
  t.after(() => {
    delete globalThis.localStorage;
    delete globalThis.sessionStorage;
  });
  t.mock.method(globalThis, "fetch", async () => selfResponse());
  assert.equal((await api.self()).username, "alice");
});

test("failed self envelope never renders identity from untrusted success data", async (t) => {
  t.mock.method(globalThis, "fetch", async () =>
    Response.json({ success: false, data: { username: "wrong" } }),
  );
  await assert.rejects(api.self(), (error) => error.status === 502);
});

test("official redirect sanitizer accepts CMH path, rejects hostile URLs, and login uses fixed destination", (t) => {
  globalThis.window = { location: { origin: "https://gateway.example" } };
  t.after(() => {
    delete globalThis.window;
  });
  assert.equal(sanitizeAuthRedirect("/cmh/whoami", "https://gateway.example"), "/cmh/whoami");
  for (const value of [
    "//evil.example/cmh/whoami",
    "https://evil.example/cmh/whoami",
    "/\\evil.example",
    "javascript:alert(1)",
  ]) {
    assert.equal(sanitizeAuthRedirect(value, "https://gateway.example"), null);
  }
  assert.equal(officialLoginUrl(), "/sign-in?redirect=%2Fcmh%2Fwhoami");
});
