import { createContext, useContext } from "react";

interface SearchContextValue {
  query: string;
  setQuery: (query: string) => void;
}

export const SearchContext = createContext<SearchContextValue | null>(null);

export function useSearch() {
  const context = useContext(SearchContext);
  if (!context) {
    throw new Error("useSearch must be used inside SearchContext.Provider");
  }
  return context;
}
