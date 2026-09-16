import { describe, expect, it } from "vitest";
import { installerRequestBody } from "./installerPayload";

describe("installerRequestBody", () => {
  it("builds a claim request without reading setup-only values", () => {
    const body = installerRequestBody("claim", { token: "one-time-token" }, "zh-TW");
    expect(body.toString()).toBe("token=one-time-token&interface_locale=zh-TW");
    expect(body.has("supported_locales")).toBe(false);
  });

  it("serializes setup locales and the optional sample choice", () => {
    const body = installerRequestBody("setup", {
      owner_email: "owner@example.test",
      supported_locales: ["en-US", "zh-TW"],
      use_sample_data: true,
      time_zone: "Asia/Taipei",
    }, "zh-TW");
    expect(body.get("supported_locales")).toBe("en-US,zh-TW");
    expect(body.get("use_sample_data")).toBe("true");
    expect(body.get("time_zone")).toBe("Asia/Taipei");
  });
});
