import localforage from "localforage";
import { assertKittySession, KITTY_MODE, kittyStorageOwner } from "@/lib/kitty";

// Resolve the authenticated owner before any operation, including module-level hydration.
export function createUserLocalStore(options: LocalForageOptions): LocalForage {
    if (!KITTY_MODE) return localforage.createInstance(options);
    const instances = new Map<string, LocalForage>();
    return new Proxy({} as LocalForage, {
        get(_target, property) {
            return async (...args: unknown[]) => {
                const owner = await kittyStorageOwner();
                assertKittySession();
                let store = instances.get(owner);
                if (!store) { store = localforage.createInstance({ ...options, name: `kitty-canvas-user-${owner}` }); instances.set(owner, store); }
                const method = store[property as keyof LocalForage] as (...values: unknown[]) => unknown;
                return method.apply(store, args);
            };
        },
    });
}
