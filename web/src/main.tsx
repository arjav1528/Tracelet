import { createRoot } from "react-dom/client";
import { App } from "./App";
import { readToken } from "./api";
import "./styles.css";

const token = readToken();
createRoot(document.getElementById("root")!).render(<App token={token} />);
