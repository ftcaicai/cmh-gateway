import assert from "node:assert/strict";
import { beforeEach, test } from "node:test";
import axios from "axios";
import { api, ApiError } from "../src/lib/api.ts";
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
