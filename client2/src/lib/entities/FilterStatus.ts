import Filter, { IFilter } from './Filter';

// This file was autogenerated. Please do not change.
// All changes will be overwrited on commit.
export interface IFilterStatus {
    enabled?: boolean;
    filters?: IFilter[];
    interval?: number;
    user_rules?: string[];
}

export default class FilterStatus {
    readonly _enabled: boolean | undefined;

    get enabled(): boolean | undefined {
        return this._enabled;
    }

    readonly _filters: Filter[] | undefined;

    get filters(): Filter[] | undefined {
        return this._filters;
    }

    readonly _interval: number | undefined;

    get interval(): number | undefined {
        return this._interval;
    }

    readonly _user_rules: string[] | undefined;

    get userRules(): string[] | undefined {
        return this._user_rules;
    }

    constructor(props: IFilterStatus) {
        if (typeof props.enabled === 'boolean') {
            this._enabled = props.enabled;
        }
        if (props.filters) {
            this._filters = props.filters.map((p) => new Filter(p));
        }
        if (typeof props.interval === 'number') {
            this._interval = props.interval;
        }
        if (props.user_rules) {
            this._user_rules = props.user_rules;
        }
    }

    serialize(): IFilterStatus {
        const data: IFilterStatus = {
        };
        if (typeof this._enabled !== 'undefined') {
            data.enabled = this._enabled;
        }
        if (typeof this._filters !== 'undefined') {
            data.filters = this._filters.map((p) => p.serialize());
        }
        if (typeof this._interval !== 'undefined') {
            data.interval = this._interval;
        }
        if (typeof this._user_rules !== 'undefined') {
            data.user_rules = this._user_rules;
        }
        return data;
    }

    validate(): string[] {
        const validate = {
            enabled: !this._enabled ? true : typeof this._enabled === 'boolean',
            interval: !this._interval ? true : typeof this._interval === 'number',
            filters: !this._filters ? true : this._filters.reduce((result, p) => result && p.validate().length === 0, true),
            user_rules: !this._user_rules ? true : this._user_rules.reduce((result, p) => result && typeof p === 'string', true),
        };
        const isError: string[] = [];
        Object.keys(validate).forEach((key) => {
            if (!(validate as any)[key]) {
                isError.push(key);
            }
        });
        return isError;
    }

    update(props: Partial<IFilterStatus>): FilterStatus {
        return new FilterStatus({ ...this.serialize(), ...props });
    }
}
