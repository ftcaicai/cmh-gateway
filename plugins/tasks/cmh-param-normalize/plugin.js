// Request processing only: no channel driver, credentials, network or shared request state.
export const meta = {
  apiVersion: 1,
  key: "cmh-param-normalize",
  name: {en: "CMH parameter normalization", zh: "CMH 参数规范化"},
  version: "1.0.0",
  author: {name: "CMH"},
  description: {en: "Normalize explicitly targeted chat request parameters", zh: "规范化明确指定的聊天请求参数"},
  configSchema: {
    type: "object",
    properties: {
      channelIds: {type: "string", title: {en: "Exact channel IDs", zh: "精确渠道 ID"}, default: "", maxLength: 2048},
      expectedModel: {type: "string", title: {en: "Expected public model", zh: "预期公开模型"}, default: "", maxLength: 128},
      expectedUpstreamModel: {type: "string", title: {en: "Expected upstream model", zh: "预期上游模型"}, default: "", maxLength: 128},
      removeTemperature: {type: "boolean", title: {en: "Remove temperature", zh: "移除 temperature"}, default: false},
      removeTopP: {type: "boolean", title: {en: "Remove top_p", zh: "移除 top_p"}, default: false},
      convertDeveloperToSystem: {type: "boolean", title: {en: "Convert developer role to system", zh: "将 developer 角色转换为 system"}, default: false},
    },
  },
};

// Keep targeting lexical: JSON numbers cannot distinguish 1, 1.0 and 1e0.
// Recheck at runtime because an older stored config can outlive a schema update.
function validatedChannelIds(config) {
  if (!config || typeof config !== "object" || Array.isArray(config)) {
    throw new Error("Config must be an object");
  }
  const allowed = ["channelIds", "expectedModel", "expectedUpstreamModel", "removeTemperature", "removeTopP", "convertDeveloperToSystem"];
  for (const key of Object.keys(config)) {
    if (!allowed.includes(key)) throw new Error("Unknown config field");
  }
  for (const key of ["removeTemperature", "removeTopP", "convertDeveloperToSystem"]) {
    if (config[key] !== undefined && typeof config[key] !== "boolean") throw new Error("Normalization switches must be boolean");
  }
  for (const key of ["expectedModel", "expectedUpstreamModel"]) {
    const value = config[key] === undefined ? "" : config[key];
    if (typeof value !== "string" || value.length > 128 || /[\s\x00-\x1f\x7f*]/.test(value)) {
      throw new Error("Expected model names must be exact strings without whitespace or wildcards");
    }
  }
  const text = config.channelIds === undefined ? "" : config.channelIds;
  if (typeof text !== "string" || text.length > 2048) throw new Error("Channel IDs must be a canonical comma-separated string");
  if (text === "") return [];
  const ids = text.split(",");
  if (ids.length > 100) throw new Error("At most 100 channel IDs are allowed");
  const seen = new Set();
  for (const id of ids) {
    if (!/^[1-9][0-9]*$/.test(id) || !Number.isSafeInteger(Number(id)) || seen.has(id)) {
      throw new Error("Channel IDs must be positive canonical unique safe integers");
    }
    seen.add(id);
  }
  if (!config.expectedModel || !config.expectedUpstreamModel) throw new Error("Targeted channels require both expected model names");
  return ids;
}

export function validateConfig(config) {
  validatedChannelIds(config);
  return null;
}

export function outbound(ctx, req) {
  if (ctx.stage !== "outbound" || ctx.format !== "openai" || ctx.path !== "/v1/chat/completions") {
    return {action: "continue"};
  }
  const config = ctx.config === undefined ? {} : ctx.config;
  let ids;
  try {
    ids = validatedChannelIds(config);
  } catch (_) {
    return {action: "reject", code: "normalize.invalid_config", message: "Invalid parameter normalization config"};
  }
  if (ids.length === 0 || !ctx.channel || !Number.isSafeInteger(ctx.channel.id) || ctx.channel.id <= 0 ||
      !ids.includes(String(ctx.channel.id)) || ctx.model !== config.expectedModel ||
      ctx.upstreamModel !== config.expectedUpstreamModel) {
    return {action: "continue"};
  }
  const body = req.body;
  if (!body || typeof body !== "object" || Array.isArray(body)) return {action: "continue"};
  const ops = [];
  if (config.removeTemperature === true && Object.prototype.hasOwnProperty.call(body, "temperature")) {
    ops.push({op: "remove", path: "/temperature"});
  }
  if (config.removeTopP === true && Object.prototype.hasOwnProperty.call(body, "top_p")) {
    ops.push({op: "remove", path: "/top_p"});
  }
  if (config.convertDeveloperToSystem === true && Array.isArray(body.messages)) {
    for (let index = 0; index < body.messages.length; index++) {
      const message = body.messages[index];
      if (message && typeof message === "object" && !Array.isArray(message) && message.role === "developer") {
        // Patch only the leaf. Never copy a message containing host media placeholders.
        ops.push({op: "replace", path: "/messages/" + index + "/role", value: "system"});
        if (ops.length > 512) {
          return {action: "reject", code: "normalize.too_many_ops", message: "Parameter normalization exceeds the operation limit"};
        }
      }
    }
  }
  if (ops.length === 0) return {action: "continue"};
  return {action: "continue", effects: [{type: "json.patch", ops}]};
}
