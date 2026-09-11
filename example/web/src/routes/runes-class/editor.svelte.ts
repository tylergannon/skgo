export class Editor {
	private words: string[];
	rev = $state(0);
	count = $derived.by(() => {
		void this.rev;
		return this.words.length;
	});

	constructor(text: string) {
		this.words = text.split(' ');
	}
}
