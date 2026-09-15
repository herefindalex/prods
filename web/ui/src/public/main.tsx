import { createRoot } from "react-dom/client";

type StoredItem = {
  kind: "catalog";
  product_id: string;
  part_number: string;
};

function AddToRFQ({ productID, partNumber }: { productID: string; partNumber: string }) {
  const add = () => {
    const stored = JSON.parse(localStorage.getItem("prods-rfq") ?? "[]") as StoredItem[];
    if (!stored.some((item) => item.product_id === productID)) {
      stored.push({ kind: "catalog", product_id: productID, part_number: partNumber });
      localStorage.setItem("prods-rfq", JSON.stringify(stored));
    }
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
