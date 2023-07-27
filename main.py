import openai
from flask import Flask, request, jsonify

class ChatGPTBotAPI:
    def __init__(self, openai_api_key):
        # Initialize OpenAI API with the provided API key
        openai.api_key = openai_api_key
        self.prompts = []

    def create_prompt(self, prompt):
        # Store the user-provided prompt for later interactions
        self.prompts.append(prompt)

    def get_response(self, prompt_index):
        # Get the response from ChatGPT bot for a given prompt index
        if prompt_index < 0 or prompt_index >= len(self.prompts):
            return "Invalid prompt index"
        
        prompt = self.prompts[prompt_index]
        response = openai.Completion.create(
            engine="text-davinci-002",  # You can choose a different engine based on your plan and requirements
            prompt=prompt,
            max_tokens=150
        )
        return response["choices"][0]["text"]

    def update_prompt(self, prompt_index, new_prompt):
        # Update an existing prompt at the given index with a new prompt
        if prompt_index < 0 or prompt_index >= len(self.prompts):
            return "Invalid prompt index"
        
        self.prompts[prompt_index] = new_prompt
        return "Prompt updated successfully"

    
    def delete_prompt(self, prompt_index):
        # Delete the prompt at the given index
        if prompt_index < 0 or prompt_index >= len(self.prompts):
            return "Invalid prompt index"

        del self.prompts[prompt_index]
        return "Prompt deleted successfully"

# Initialize the Flask app
app = Flask(__name__)

# Initialize the ChatGPTBotAPI with your OpenAI API key
openai_api_key = "YOUR_CHATGPT_API_KEY_HERE"
chatbot_api = ChatGPTBotAPI(openai_api_key)

# API endpoints
@app.route('/create', methods=['POST'])
def create_prompt():
    data = request.get_json()
    prompt = data.get('prompt')
    if not prompt:
        return jsonify({"error": "Prompt not provided"}), 400
    
    chatbot_api.create_prompt(prompt)
    return jsonify({"message": "Prompt created successfully"}), 201

@app.route('/get/<int:prompt_index>', methods=['GET'])
def get_response(prompt_index):
    response = chatbot_api.get_response(prompt_index)
    return jsonify({"response": response}), 200

@app.route('/delete/<int:prompt_index>', methods=['DELETE'])
def delete_prompt(prompt_index):
    response = chatbot_api.delete_prompt(prompt_index)
    return jsonify({"message": response}), 200

@app.route('/update/<int:prompt_index>', methods=['PUT'])
def update_prompt(prompt_index):
    data = request.get_json()
    new_prompt = data.get('new_prompt')
    if not new_prompt:
        return jsonify({"error": "New prompt not provided"}), 400

    response = chatbot_api.update_prompt(prompt_index, new_prompt)
    return jsonify({"message": response}), 200

# Run the app
if __name__ == '__main__':
    app.run(debug=True)
