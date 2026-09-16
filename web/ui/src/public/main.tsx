import { useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { createRoot } from "react-dom/client";

type StoredItem = {
  kind: "catalog";
  product_id: string;
  part_number: string;
};

const storageKey = "prods-rfq";

function readStored(): StoredItem[] {
  try {
    const value = JSON.parse(localStorage.getItem(storageKey) ?? "[]") as unknown;
    if (!Array.isArray(value)) return [];
    const seen = new Set<string>();
    return value.filter((item): item is StoredItem => {
      if (!item || typeof item !== "object") return false;
      const candidate = item as Partial<StoredItem>;
      if (
        candidate.kind !== "catalog" ||
        typeof candidate.product_id !== "string" ||
        typeof candidate.part_number !== "string" ||
        seen.has(candidate.product_id)
      ) return false;
      seen.add(candidate.product_id);
      return true;
    });
  } catch {
    return [];
  }
}

function store(items: StoredItem[]): void {
  localStorage.setItem(storageKey, JSON.stringify(items));
}

function AddToRFQ({ productID, partNumber }: { productID: string; partNumber: string }) {
  const add = () => {
    const stored = readStored();
    if (!stored.some((item) => item.product_id === productID)) {
      stored.push({ kind: "catalog", product_id: productID, part_number: partNumber });
    }
    store(stored);
    window.location.assign(`/rfq?product_id=${encodeURIComponent(productID)}`);
  };
  return <button onClick={add}>Add to RFQ list</button>;
}

const island = document.getElementById("rfq-island");
if (island?.dataset.productId && island.dataset.partNumber) {
  createRoot(island).render(
    <AddToRFQ productID={island.dataset.productId} partNumber={island.dataset.partNumber} />,
  );
}

type SelectionSlot = { element: HTMLElement; item: StoredItem };
type MoreSlot = { element: HTMLElement; productID: string; row: HTMLTableRowElement };

function ListingRFQ({ root }: { root: HTMLElement }) {
  const selectionSlots = useMemo<SelectionSlot[]>(() =>
    [...document.querySelectorAll<HTMLElement>("[data-rfq-select]")].flatMap((element) => {
      const productID = element.dataset.rfqSelect;
      const partNumber = element.dataset.partNumber;
      return productID && partNumber
        ? [{ element, item: { kind: "catalog" as const, product_id: productID, part_number: partNumber } }]
        : [];
    }), []);
  const moreSlots = useMemo<MoreSlot[]>(() =>
    [...document.querySelectorAll<HTMLElement>("[data-row-more]")].flatMap((element) => {
      const productID = element.dataset.rowMore;
      const row = productID
        ? document.querySelector<HTMLTableRowElement>(`tr[data-product-row="${CSS.escape(productID)}"]`)
        : null;
      if (!productID || !row) return [];
      row.id = `product-row-${productID}`;
      return [{ element, productID, row }];
    }), []);
  const [selected, setSelected] = useState<StoredItem[]>(readStored);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());

  useEffect(() => {
    if (!window.matchMedia("(max-width: 1024px)").matches) return;
    document.querySelectorAll<HTMLDetailsElement>(".catalog-disclosure[open]")
      .forEach((details) => details.removeAttribute("open"));
  }, []);

  const selectLabel = root.dataset.selectLabel ?? "Select for RFQ";
  const selectedLabel = root.dataset.selectedLabel ?? "Selected products";
  const clearLabel = root.dataset.clearLabel ?? "Clear selection";
  const requestLabel = root.dataset.requestLabel ?? "Request quote";
  const moreLabel = root.dataset.moreLabel ?? "More details";
  const lessLabel = root.dataset.lessLabel ?? "Less details";

  const update = (item: StoredItem, checked: boolean) => {
    const next = checked
      ? [...selected.filter((candidate) => candidate.product_id !== item.product_id), item]
      : selected.filter((candidate) => candidate.product_id !== item.product_id);
    setSelected(next);
    store(next);
  };
  const clear = () => {
    setSelected([]);
    store([]);
  };
  const requestURL = useMemo(() => {
    const query = new URLSearchParams({ lang: document.documentElement.lang });
    selected.forEach((item) => query.append("product_id", item.product_id));
    return `/rfq?${query.toString()}`;
  }, [selected]);

  return <>
    {selectionSlots.map(({ element, item }) => createPortal(
      <label className="rfq-select-control">
        <input
          type="checkbox"
          checked={selected.some((candidate) => candidate.product_id === item.product_id)}
          onChange={(event) => update(item, event.currentTarget.checked)}
        />
        <span>{selectLabel}</span>
      </label>,
      element,
      item.product_id,
    ))}
    {moreSlots.map(({ element, productID, row }) => createPortal(
      <button
        className="row-more-control"
        type="button"
        aria-expanded={expanded.has(productID)}
        aria-controls={row.id}
        onClick={() => {
          const next = new Set(expanded);
          if (next.has(productID)) next.delete(productID); else next.add(productID);
          row.classList.toggle("is-expanded", next.has(productID));
          setExpanded(next);
        }}
      >{expanded.has(productID) ? lessLabel : moreLabel}</button>,
      element,
      productID,
    ))}
    {selected.length > 0 ? <aside className="rfq-selection-tray" aria-live="polite">
      <span><strong>{selected.length}</strong> {selectedLabel}</span>
      <a className="button" href={requestURL}>{requestLabel}</a>
      <button type="button" onClick={clear}>{clearLabel}</button>
    </aside> : null}
  </>;
}

const listingIsland = document.getElementById("listing-rfq-island");
if (listingIsland) createRoot(listingIsland).render(<ListingRFQ root={listingIsland} />);
