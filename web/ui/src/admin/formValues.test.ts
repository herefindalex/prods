import { describe, expect, it } from "vitest";

import { replaceFormValues } from "./formValues";

type Values = {
  organization?: {
    display_name?: string;
    legal_name?: string;
  };
};

describe("replaceFormValues", () => {
  it("clears fields omitted by a complete server response", () => {
    let current: Values = {
      organization: { display_name: "RS Group", legal_name: "RS Components Ltd." },
    };
    const form = {
      resetFields() {
        current = {};
      },
      setFieldsValue(values: Values) {
        current = {
          ...current,
          ...values,
          organization: { ...current.organization, ...values.organization },
        };
      },
    };

    replaceFormValues(form, { organization: { display_name: "TME" } });

    expect(current).toEqual({ organization: { display_name: "TME" } });
  });
});
