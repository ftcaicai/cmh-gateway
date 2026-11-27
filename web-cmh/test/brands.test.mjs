import assert from "node:assert/strict";
import { beforeEach, test } from "node:test";
import axios from "axios";
import { api, ApiError, ownsBrand } from "../src/lib/api.ts";
import { customerNoteSchema } from "../src/features/customers/lib/schema.ts";
import { applyAuthBundle, clearAuthentication } from "@/lib/auth-session";

beforeEach(() => {
  clearAuthentication(false, "idle");
  const expires = Math.floor(Date.now() / 1000) + 3600;
  applyAuthBundle(
    {
      access_token: "synthetic",
      token_type: "Bearer",
      access_expires_at: expires,
      user: { id: 1, username: "owner", role: 10 },
      session: {
        sid: "oem-session",
        current: true,
        login_method: "password",
        ip: "",
        user_agent: "",
        created_at: 1,
        last_active_at: 1,
        expires_at: expires,
      },
    },
    false,
  );
  axios.defaults.adapter = async () => {
    throw new Error("Unexpected refresh");
  };
});

test("ownership hints grant controls only for exact owner and reject unknown/missing access", () => {
  assert.equal(ownsBrand({ access: "owner" }), true);
  for (const value of [null, undefined, {}, { access: "readonly" }, { access: "admin" }])
    assert.equal(ownsBrand(value), false);
});

test("team invite and self acceptance send only explicit target or token", async (t) => {
  const seen = [];
  t.mock.method(globalThis, "fetch", async (path, options) => {
    seen.push([path, options.method, JSON.parse(options.body)]);
    return Response.json({ success: true, data: { token: "synthetic", invitation: { id: "i" } } });
  });
  await api.inviteTeamMember("site/1", 9);
  await api.acceptTeamInvitation("synthetic");
  assert.deepEqual(seen, [
    ["/api/ext/oem/sites/site%2F1/team-invitations", "POST", { target_user_id: 9 }],
    ["/api/ext/oem/team-invitations/accept", "POST", { token: "synthetic" }],
  ]);
});

test("team revoke sends exact CAS version once on conflict", async (t) => {
  let calls = 0;
  t.mock.method(globalThis, "fetch", async (path, options) => {
    calls++;
    assert.equal(path, "/api/ext/oem/sites/site-1/team/9/revoke");
    assert.deepEqual(JSON.parse(options.body), { version: 3 });
    return new Response(null, { status: 409 });
  });
  await assert.rejects(
    api.revokeTeamMember("site-1", 9, 3),
    (error) => error instanceof ApiError && error.status === 409,
  );
  assert.equal(calls, 1);
});

test("team self acceptance does not replay token after unauthorized response", async (t) => {
  let calls = 0;
  t.mock.method(globalThis, "fetch", async () => {
    calls++;
    return new Response(null, { status: 401 });
  });
  await assert.rejects(
    api.acceptTeamInvitation("synthetic"),
    (error) => error instanceof ApiError && error.status === 401,
  );
  assert.equal(calls, 1);
});

test("note form permits plain LF/tab without trimming and bounds UTF-8 bytes", () => {
  for (const note of ["", "  text\n\t  ", "界".repeat(666) + "ab", "a".repeat(2000)]) {
    assert.deepEqual(customerNoteSchema.parse({ note }), { note });
  }
  for (const note of ["界".repeat(667), "a".repeat(2001), "x\r", "x\u0000", "x\u007f", "x\u0085"]) {
    assert.equal(customerNoteSchema.safeParse({ note }).success, false);
  }
});

test("private notes read uses site/customer scope and retains exact plain text", async (t) => {
  const data = { note: "  private\n\t<script>plain</script>  ", version: 0, updated_at: 0 };
  t.mock.method(globalThis, "fetch", async (path, options) => {
    assert.equal(path, "/api/ext/oem/sites/site%2F1/customers/2/notes");
    assert.equal(options.method, "GET");
    assert.equal(options.body, undefined);
    return Response.json({ success: true, data });
  });
  assert.deepEqual(await api.customerNote("site/1", 2), data);
});

test("private note writes only note/version once and preserves CAS conflicts", async (t) => {
  let calls = 0;
  t.mock.method(globalThis, "fetch", async (path, options) => {
    calls++;
    assert.equal(path, "/api/ext/oem/sites/site-1/customers/2/notes");
    assert.equal(options.method, "PUT");
    assert.deepEqual(JSON.parse(options.body), { note: "  private\n\t", version: 0 });
    return new Response(null, { status: 409 });
  });
  await assert.rejects(
    api.updateCustomerNote("site-1", 2, 0, "  private\n\t"),
    (error) => error instanceof ApiError && error.status === 409,
  );
  assert.equal(calls, 1);
});

test("private note 401 never refreshes or replays text", async (t) => {
  let calls = 0;
  t.mock.method(globalThis, "fetch", async () => {
    calls++;
    return new Response(null, { status: 401 });
  });
  await assert.rejects(
    api.updateCustomerNote("site-1", 2, 1, "private"),
    (error) => error instanceof ApiError && error.status === 401,
  );
  assert.equal(calls, 1);
});

test("customer invitation sends only target ID and accepts one-time response", async (t) => {
  t.mock.method(globalThis, "fetch", async (path, options) => {
    assert.equal(path, "/api/ext/oem/sites/site-1/customer-invitations");
    assert.equal(options.method, "POST");
    assert.deepEqual(JSON.parse(options.body), { target_user_id: 2 });
    return Response.json({
      success: true,
      data: { invitation: { id: "invite-1" }, token: "synthetic-token" },
    });
  });
  const result = await api.inviteCustomer("site-1", 2);
  assert.equal(result.token, "synthetic-token");
});

test("self membership accepts null nested association without merchant site input", async (t) => {
  t.mock.method(globalThis, "fetch", async (path) => {
    assert.equal(path, "/api/ext/oem/customer-membership");
    return Response.json({ success: true, data: { membership: null } });
  });
  assert.deepEqual(await api.membership(), { membership: null });
});

test("accept invitation 401 never refreshes or replays the secret", async (t) => {
  let calls = 0;
  t.mock.method(globalThis, "fetch", async (path, options) => {
    calls++;
    assert.equal(path, "/api/ext/oem/customer-invitations/accept");
    assert.deepEqual(JSON.parse(options.body), { token: "synthetic-secret" });
    return new Response(null, { status: 401 });
  });
  await assert.rejects(
    api.acceptInvitation("synthetic-secret"),
    (error) => error instanceof ApiError && error.status === 401,
  );
  assert.equal(calls, 1);
});

test("catalog reports actual publication and policy flags without mutation", async (t) => {
  t.mock.method(globalThis, "fetch", async (path, options) => {
    assert.equal(path, "/api/ext/oem/catalog");
    assert.equal(options.method, "GET");
    assert.equal(options.body, undefined);
    return Response.json({
      success: true,
      data: {
        models: ["owned-model"],
        model_policy_enabled: true,
        destination_policy_enabled: false,
      },
    });
  });
  assert.deepEqual(await api.catalog(), {
    models: ["owned-model"],
    model_policy_enabled: true,
    destination_policy_enabled: false,
  });
});

test("customer audits use selected site endpoint without token fields", async (t) => {
  t.mock.method(globalThis, "fetch", async (path) => {
    assert.equal(path, "/api/ext/oem/sites/site-1/customer-audits");
    return Response.json({
      success: true,
      data: [{ id: "audit-1", action: "customer.invited", user_id: 2, created_at: 1 }],
    });
  });
  assert.deepEqual(await api.customerAudits("site-1"), [
    { id: "audit-1", action: "customer.invited", user_id: 2, created_at: 1 },
  ]);
});

test("brand list uses real management endpoint with memory Bearer", async (t) => {
  t.mock.method(globalThis, "fetch", async (path, options) => {
    assert.equal(path, "/api/ext/oem/sites");
    assert.equal(options.headers.Authorization, "Bearer synthetic");
    assert.equal(options.cache, "no-store");
    return Response.json({ success: true, data: [] });
  });
  assert.deepEqual(await api.brands(), []);
});

test("save sends version CAS exactly once and reports conflicts", async (t) => {
  let calls = 0;
  t.mock.method(globalThis, "fetch", async (path, options) => {
    calls++;
    assert.equal(path, "/api/ext/oem/sites/site-1");
    assert.equal(options.method, "PUT");
    assert.deepEqual(JSON.parse(options.body), {
      name: "Brand",
      support_email: "",
      description: "",
      version: 2,
    });
    return Response.json({ success: false, code: "OEM_VERSION_CONFLICT" }, { status: 409 });
  });
  await assert.rejects(
    api.updateBrand("site-1", 2, { name: "Brand", support_email: "", description: "" }),
    (error) => error instanceof ApiError && error.status === 409,
  );
  assert.equal(calls, 1);
});

test("create and update strip server-owned fields from structurally extended profile inputs", async (t) => {
  const site = {
    id: "server-id",
    name: "Brand",
    support_email: "support@example.test",
    description: "Owned",
    version: 8,
    created_at: 1,
    updated_at: 2,
  };
  const seen = [];
  t.mock.method(globalThis, "fetch", async (_path, options) => {
    seen.push(JSON.parse(options.body));
    return Response.json({ success: true, data: site });
  });
  await api.createBrand(site);
  await api.updateBrand(site.id, 8, site);
  assert.deepEqual(seen, [
    { name: "Brand", support_email: "support@example.test", description: "Owned" },
    { name: "Brand", support_email: "support@example.test", description: "Owned", version: 8 },
  ]);
});

test("mutation 401 is not refreshed or replayed", async (t) => {
  let calls = 0;
  t.mock.method(globalThis, "fetch", async () => {
    calls++;
    return new Response(null, { status: 401 });
  });
  await assert.rejects(
    api.createBrand({ name: "Brand", support_email: "", description: "" }),
    (error) => error instanceof ApiError && error.status === 401,
  );
  assert.equal(calls, 1);
});

test("permission failure remains an error rather than an empty list", async (t) => {
  t.mock.method(globalThis, "fetch", async () => new Response(null, { status: 403 }));
  await assert.rejects(api.brands(), (error) => error instanceof ApiError && error.status === 403);
});

test("audit request uses selected profile and validates envelope", async (t) => {
  t.mock.method(globalThis, "fetch", async (path) => {
    assert.equal(path, "/api/ext/oem/sites/site-1/audits");
    return Response.json({ success: false, data: [] });
  });
  await assert.rejects(
    api.brandAudits("site-1"),
    (error) => error instanceof ApiError && error.status === 502,
  );
});

test("paged lists send offset/limit only when set and keep the first page bare", async (t) => {
  const seen = [];
  t.mock.method(globalThis, "fetch", async (path) => {
    seen.push(path);
    return Response.json({ success: true, data: [] });
  });
  await api.brandAudits("site-1");
  await api.brandAudits("site-1", { offset: 50, limit: 50 });
  await api.customers("site-1", { offset: 100 });
  await api.teamAudits("site-1", { limit: 20 });
  assert.deepEqual(seen, [
    "/api/ext/oem/sites/site-1/audits",
    "/api/ext/oem/sites/site-1/audits?offset=50&limit=50",
    "/api/ext/oem/sites/site-1/customers?offset=100",
    "/api/ext/oem/sites/site-1/team-audits?limit=20",
  ]);
});

test("storefront pause sends only the boolean and preview encodes theme and page", async (t) => {
  const calls = [];
  t.mock.method(globalThis, "fetch", async (path, init) => {
    calls.push([path, init.method, init.body]);
    return Response.json({ success: true, data: { paused: true, paused_at: 1, html: "<p>x</p>" } });
  });
  await api.setBrandPause("site 1", true);
  await api.brandPreview("site-1", "technology", "docs");
  await api.restoreLogo("site-1", "entry/1");
  assert.deepEqual(calls, [
    ["/api/ext/oem/sites/site%201/pause", "PUT", JSON.stringify({ paused: true })],
    ["/api/ext/oem/sites/site-1/preview?theme=technology&page=docs", "GET", undefined],
    ["/api/ext/oem/sites/site-1/logo-history/entry%2F1/restore", "POST", "{}"],
  ]);
});
