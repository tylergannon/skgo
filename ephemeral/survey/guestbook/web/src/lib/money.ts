export class Money {
	constructor(
		readonly cents: number,
		readonly currency: string
	) {}

	format() {
		return `${this.currency} ${(this.cents / 100).toFixed(2)}`;
	}
}
