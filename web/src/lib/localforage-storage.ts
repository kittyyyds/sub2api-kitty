import { createUserLocalStore } from "@/lib/user-localforage";
import { KITTY_MODE } from "@/lib/kitty";
import type { StateStorage } from "zustand/middleware";

const localforage = createUserLocalStore({
    name: "infinite-canvas",
    storeName: "app_state",
});

export const localForageStorage: StateStorage = {
    getItem: async (name) => {
        if (typeof window === "undefined") return null;
        try {
            return (await localforage.getItem<string>(name)) || null;
        } catch (error) {
            if (KITTY_MODE) throw error;
            return window.localStorage.getItem(name);
        }
    },
    setItem: async (name, value) => {
        if (typeof window === "undefined") return;
        try {
            await localforage.setItem(name, value);
        } catch (error) {
            if (KITTY_MODE) throw error;
            window.localStorage.setItem(name, value);
        }
    },
    removeItem: async (name) => {
        if (typeof window === "undefined") return;
        try {
            await localforage.removeItem(name);
        } catch (error) {
            if (KITTY_MODE) throw error;
            window.localStorage.removeItem(name);
        }
    },
};
